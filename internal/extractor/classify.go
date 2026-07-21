package extractor

import (
	"regexp"
	"strings"
	"unicode"

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

// promptishRe matches identifiers that conventionally hold prompts. "system"
// is handled separately in isPromptish: as a substring it drowns in compound
// words (FileSystemError, subsystem), so it only counts as the identifier's
// first word (system, systemPrompt, system_message).
var promptishRe = regexp.MustCompile(`(?i)prompt|instruction|persona`)

// isPromptish reports whether an identifier segment conventionally names a
// prompt value.
func isPromptish(segment string) bool {
	if promptishRe.MatchString(segment) {
		return true
	}
	words := identWords(segment)
	return len(words) > 0 && words[0] == "system"
}

// identWords splits an identifier into lowercase words on underscores,
// hyphens, digits and camelCase boundaries: "FileSystemError" yields
// ["file", "system", "error"].
func identWords(s string) []string {
	var words []string
	var cur []rune
	runes := []rune(s)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r):
			flush()
		case unicode.IsUpper(r):
			prevLower := i > 0 && unicode.IsLower(runes[i-1])
			acronymEnd := i > 0 && unicode.IsUpper(runes[i-1]) &&
				i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLower || acronymEnd {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// segmentRe extracts identifier segments from a binding name, so that
// "$this->systemPrompt" yields "systemPrompt" and "prompt.required_without"
// yields "required_without". Deny and promptish decisions look at the LAST
// segment: it is the one that names the value ("prompt.hint" is a hint).
// Case is preserved so that camelCase word boundaries survive.
var segmentRe = regexp.MustCompile(`[A-Za-z0-9_-]+`)

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
	// presentation: styling values, never prompts
	"style": true, "styles": true, "class": true, "classname": true,
	"classes": true, "css": true, "icon": true,
	// database artifacts ("query" stays allowed: in LLM code a userQuery is
	// often real model input, and SQL content is caught by looksLikeSQL)
	"sql": true, "statement": true, "stmt": true, "columns": true,
}

// normalizeBinding strips quotes and sigils from a binding name so that
// '$systemPrompt', '"prompt"' and 'prompt' compare equal. Case is preserved
// for camelCase analysis; callers lowercase where needed.
func normalizeBinding(s string) string {
	return strings.Trim(s, "'\"$ \t")
}

// denyCallees are call sites whose string arguments are never prompts:
// test assertions hold expected MODEL OUTPUT, loggers and consoles hold
// diagnostics, UI input boxes hold labels, database handles hold SQL,
// styled-components hold CSS. Matching is case-insensitive substring on the
// rendered callee; the call nearest to the string literal decides.
var denyCallees = []string{
	// test assertions: expected model output, not model input
	"expect(", "->tobe", "->tocontain", "->toequal", "->tomatch", "->tothrow",
	".tobe(", ".tocontain", ".toequal", ".tomatch", ".tostrictequal",
	"assertequals", "assertsame", "assertcontains", "assertstringcontains",
	"assertmatch",
	// diagnostics
	"console.", "logger.", "log.error", "log.warn", "log.info", "log.debug",
	"log.log", "log.trace",
	// UI copy
	"showinputbox", "showquickpick", "showinformationmessage",
	"showerrormessage", "showwarningmessage", "addhelptext", "styled.",
	// database handles
	"db.", "database.", ".prepare(",
}

// denyExactCallees are denied only on exact match: the string argument is a
// question shown to the user, not model input.
var denyExactCallees = map[string]bool{"input": true, "print": true}

// isDeniedCallee reports whether a rendered callee is a deny-listed call or
// an error/exception constructor ("new PrismException", "RuntimeError").
func isDeniedCallee(callee string) bool {
	c := strings.ToLower(strings.ReplaceAll(callee, " ", ""))
	if strings.HasSuffix(c, "error") || strings.HasSuffix(c, "exception") {
		return true
	}
	if denyExactCallees[c] {
		return true
	}
	for _, pattern := range denyCallees {
		if strings.Contains(c, pattern) {
			return true
		}
	}
	return false
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
			if isDeniedCallee(callee) {
				return Low, "", notPrompt
			}
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

		// A prompt-like enclosing function, method or class name is evidence
		// too: a heredoc returned by StoryGroupPromptBuilder::buildSystemMessage
		// is a prompt even when nothing else names it. Error and exception
		// scopes veto instead (their strings are diagnostics), and assertion
		// helpers (assertPrompt, expectPrompt) carry no evidence.
		if field, ok := lang.declKinds[kind]; ok {
			if name := fieldText(p, field, src); name != "" {
				lower := strings.ToLower(name)
				if strings.HasSuffix(lower, "error") || strings.HasSuffix(lower, "exception") {
					return Low, "", notPrompt
				}
				// Assertion helpers hold expected output, never prompts.
				if words := identWords(name); len(words) > 0 &&
					(words[0] == "assert" || words[0] == "expect") {
					return Low, "", notPrompt
				}
				if isPromptish(name) && mediumContext == "" {
					mediumContext = "in " + name
				}
			}
		}

		if nameNode != nil {
			if child.Id() == nameNode.Id() {
				return Low, "", notPrompt // the string IS a key, not a value
			}
			raw := nameNode.Utf8Text(src)
			if segment := lastIdentSegment(raw); segment != "" {
				// Deny decisions belong to the binding nearest to the
				// literal; prompt-like evidence is accepted at any depth.
				// The identifier's LAST word decides the deny: a
				// PromptInputTabLabel is a label, whatever its prefix.
				if !sawNearestBinding {
					sawNearestBinding = true
					words := identWords(segment)
					if denyKeys[strings.ToLower(segment)] ||
						(len(words) > 0 && denyKeys[words[len(words)-1]]) {
						return Low, "", notPrompt
					}
				}
				if isPromptish(segment) && mediumContext == "" {
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
