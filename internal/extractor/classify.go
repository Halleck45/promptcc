package extractor

import (
	"regexp"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// sdkCallees are substrings identifying known LLM SDK call sites. Matching
// is case-insensitive on the rendered callee (e.g. "client.messages.create",
// "$client->chat()->create", "new PromptTemplate"). The list is curated and
// documented in the README; additions are welcome.
var sdkCallees = []string{
	// Anthropic
	"messages.create", "messages.stream", "messages()->create",
	// OpenAI
	"chat.completions.create", "completions.create", "responses.create",
	"chat()->create", "responses()->create", "completions()->create",
	// Google
	"generatecontent", "generativemodel",
	// Vercel AI SDK
	"generatetext", "streamtext", "generateobject", "streamobject",
	// LangChain
	"chatprompttemplate", "prompttemplate", "from_template", "from_messages",
	"fromtemplate", "frommessages",
	// AWS Bedrock
	"invoke_model", "invokemodel", "converse(",
	// Ollama / generic
	"ollama.chat", "ollama.generate",
}

// promptishRe matches identifiers that conventionally hold prompts.
var promptishRe = regexp.MustCompile(`(?i)prompt|system|instruction|persona`)

// denyKeys are argument and object keys whose string values are never
// prompts, even inside an SDK call: model ids, roles, urls, api keys...
// Only the key nearest to the string literal is considered.
var denyKeys = map[string]bool{
	"model": true, "role": true, "name": true, "id": true, "type": true,
	"url": true, "uri": true, "path": true, "file": true, "filename": true,
	"version": true, "format": true, "encoding": true, "method": true,
	"key": true, "api_key": true, "apikey": true, "token": true,
	"lang": true, "language": true,
}

// maxClimb bounds the ancestor walk during classification.
const maxClimb = 15

// classify walks up from a string node looking for evidence that it is a
// prompt: an enclosing SDK call (high), or a prompt-like binding name
// (medium). It returns ok=false when no evidence is found, or when the
// string is provably not a prompt (a key, or a denylisted value).
func classify(lang *language, n *sitter.Node, src []byte) (Confidence, string, bool) {
	var mediumContext string
	sawNearestKey := false
	child := n
	for p, depth := n.Parent(), 0; p != nil && depth < maxClimb; p, depth = p.Parent(), depth+1 {
		kind := p.Kind()

		if render, ok := lang.callKinds[kind]; ok {
			callee := render(p, src)
			if isSDKCallee(callee) {
				return High, "call " + compactCallee(callee), true
			}
		}

		var keyNode *sitter.Node
		if field, ok := lang.keyedKinds[kind]; ok {
			keyNode = p.ChildByFieldName(field)
		} else if kind == "array_element_initializer" && p.NamedChildCount() >= 2 {
			// PHP array pairs ('system' => "...") have no field names, and
			// keyless list items ([$a, $b]) have a single named child.
			keyNode = p.NamedChild(0)
		}
		if keyNode != nil {
			if child.Id() == keyNode.Id() {
				return Low, "", false // the string IS a key, not a value
			}
			if !sawNearestKey {
				sawNearestKey = true
				key := strings.ToLower(strings.Trim(keyNode.Utf8Text(src), `'"`))
				if denyKeys[key] {
					return Low, "", false
				}
				if promptishRe.MatchString(key) && mediumContext == "" {
					mediumContext = "arg " + key
				}
			}
		}

		if field, ok := lang.assignmentKinds[kind]; ok {
			// Only the right-hand side of an assignment is a value.
			if left := p.ChildByFieldName(field); left != nil && child.Id() != left.Id() {
				if name := left.Utf8Text(src); promptishRe.MatchString(name) && mediumContext == "" {
					mediumContext = "var " + name
				}
			}
		}

		child = p
	}
	if mediumContext != "" {
		return Medium, mediumContext, true
	}
	return Low, "", false
}

func isSDKCallee(callee string) bool {
	c := strings.ToLower(strings.ReplaceAll(callee, " ", ""))
	for _, pattern := range sdkCallees {
		if strings.Contains(c, pattern) {
			return true
		}
	}
	return false
}

// compactCallee trims noisy callee renderings (long chains, newlines).
func compactCallee(callee string) string {
	callee = strings.Join(strings.Fields(callee), "")
	if len(callee) > 60 {
		callee = "..." + callee[len(callee)-57:]
	}
	return callee
}
