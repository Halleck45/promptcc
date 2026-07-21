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

// segmentRe extracts identifier segments from a binding name, so that
// "$this->systemPrompt" yields "systemprompt" and "prompt.required_without"
// yields "required_without". Deny and promptish decisions look at the LAST
// segment: it is the one that names the value ("prompt.hint" is a hint).
var segmentRe = regexp.MustCompile(`[a-z0-9_-]+`)

func lastIdentSegment(s string) string {
	segments := segmentRe.FindAllString(normalizeBinding(s), -1)
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

// denyKeys are binding names (argument keys, object keys, variable and
// property names) whose string values are never prompts: model ids, roles,
// urls, api keys, and human-facing documentation fields. Only the binding
// nearest to the string literal is considered.
var denyKeys = map[string]bool{
	"model": true, "role": true, "name": true, "id": true, "type": true,
	"url": true, "uri": true, "path": true, "file": true, "filename": true,
	"version": true, "format": true, "encoding": true, "method": true,
	"key": true, "api_key": true, "apikey": true, "token": true,
	"lang": true, "language": true,
	// documentation fields: prose about the code, not prompts for a model
	"description": true, "summary": true, "notes": true, "note": true,
	"comment": true, "comments": true, "doc": true, "docs": true,
	"help": true, "usage": true, "label": true, "title": true,
	"caption": true, "hint": true, "placeholder": true, "alt": true,
	"signature": true, "slug": true,
}

// normalizeBinding lowercases a binding name and strips quotes and sigils
// so that '$systemPrompt', '"prompt"' and 'prompt' compare equal.
func normalizeBinding(s string) string {
	return strings.Trim(strings.ToLower(s), "'\"$ \t")
}

// maxClimb bounds the ancestor walk during classification.
const maxClimb = 15

// verdict is the outcome of classification. The distinction between
// noEvidence and notPrompt matters: a string with no evidence may still be
// rescued by the prose heuristic, a vetoed string may not.
type verdict int

const (
	noEvidence verdict = iota // nothing found, heuristic may apply
	isPrompt                  // positive evidence (SDK call, prompt-like name)
	notPrompt                 // definitive veto (key node, denylisted binding)
)

// classify walks up from a string node looking for evidence that it is a
// prompt: an enclosing SDK call (high), or a prompt-like binding name
// (medium).
func classify(lang *language, n *sitter.Node, src []byte) (Confidence, string, verdict) {
	var mediumContext string
	sawNearestBinding := false
	child := n
	for p, depth := n.Parent(), 0; p != nil && depth < maxClimb; p, depth = p.Parent(), depth+1 {
		kind := p.Kind()

		if render, ok := lang.callKinds[kind]; ok {
			callee := render(p, src)
			if isSDKCallee(callee) {
				return High, "call " + compactCallee(callee), isPrompt
			}
		}

		var nameNode *sitter.Node
		prefix := "arg "
		if field, ok := lang.keyedKinds[kind]; ok {
			nameNode = p.ChildByFieldName(field)
		} else if kind == "array_element_initializer" || kind == "property_element" || kind == "argument" {
			// PHP array pairs ('system' => "..."), property declarations
			// (protected $prompt = ...) and named arguments (notes: "...")
			// have no field names; their name is the first named child.
			// Keyless variants ([$a, $b], positional arguments) have a
			// single named child and carry no name evidence.
			if p.NamedChildCount() >= 2 {
				nameNode = p.NamedChild(0)
				if kind == "property_element" {
					prefix = "var "
				}
			}
		} else if field, ok := lang.assignmentKinds[kind]; ok {
			// Only the right-hand side of an assignment is a value.
			if left := p.ChildByFieldName(field); left != nil && child.Id() != left.Id() {
				nameNode = left
				prefix = "var "
			}
		}

		if nameNode != nil {
			if child.Id() == nameNode.Id() {
				return Low, "", notPrompt // the string IS a key, not a value
			}
			raw := nameNode.Utf8Text(src)
			if name := lastIdentSegment(raw); name != "" {
				// Deny decisions belong to the binding nearest to the
				// literal; prompt-like evidence is accepted at any depth.
				if !sawNearestBinding {
					sawNearestBinding = true
					if denyKeys[name] {
						return Low, "", notPrompt
					}
				}
				if promptishRe.MatchString(name) && mediumContext == "" {
					mediumContext = prefix + raw
				}
			}
		}

		child = p
	}
	if mediumContext != "" {
		return Medium, mediumContext, isPrompt
	}
	return Low, "", noEvidence
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
