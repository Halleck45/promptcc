package extractor

import (
	"fmt"
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// worker owns one parser per grammar. Parsers are not safe for concurrent
// use, so each goroutine gets its own worker.
type worker struct {
	parsers map[string]*sitter.Parser
}

func newWorker() *worker {
	return &worker{parsers: map[string]*sitter.Parser{}}
}

func (w *worker) close() {
	for _, p := range w.parsers {
		p.Close()
	}
}

func (w *worker) parserFor(lang *language) (*sitter.Parser, error) {
	if p, ok := w.parsers[lang.name]; ok {
		return p, nil
	}
	p := sitter.NewParser()
	if err := p.SetLanguage(sitter.NewLanguage(lang.ptr)); err != nil {
		p.Close()
		return nil, fmt.Errorf("loading %s grammar: %w", lang.name, err)
	}
	w.parsers[lang.name] = p
	return p, nil
}

func (w *worker) extractFile(path string) ([]Prompt, error) {
	lang := languageFor(path)
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parser, err := w.parserFor(lang)
	if err != nil {
		return nil, err
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("%s: parse failed", path)
	}
	defer tree.Close()

	var prompts []Prompt
	var visit func(n *sitter.Node)
	visit = func(n *sitter.Node) {
		if lang.stringRoots[n.Kind()] {
			if p, ok := w.extractString(lang, n, src, path); ok {
				prompts = append(prompts, p)
			}
			return // never descend into a string root
		}
		for i := uint(0); i < n.NamedChildCount(); i++ {
			visit(n.NamedChild(i))
		}
	}
	visit(tree.RootNode())
	return prompts, nil
}

// extractString decodes a string literal and classifies it as a prompt.
func (w *worker) extractString(lang *language, n *sitter.Node, src []byte, path string) (Prompt, bool) {
	text, slots := decodeString(lang, n, src)
	confidence, context, ok := classify(lang, n, src)
	if !ok {
		if !looksLikeProse(text) {
			return Prompt{}, false
		}
		confidence, context = Low, "heuristic: natural language literal"
	}
	// A prompt-like name is not enough: the value itself must read like
	// natural-language instructions ('prompt' => 'required|string' does not).
	if confidence == Medium && !looksLikeInstruction(text) {
		return Prompt{}, false
	}
	if len([]rune(text)) < minLength(confidence) {
		return Prompt{}, false
	}
	return Prompt{
		File:       path,
		Line:       int(n.StartPosition().Row) + 1,
		EndLine:    int(n.EndPosition().Row) + 1,
		Context:    context,
		Confidence: confidence,
		Text:       text,
		Slots:      slots,
	}, true
}

// minLength filters out short strings that carry no analyzable behavior.
func minLength(c Confidence) int {
	switch c {
	case High:
		return 12
	case Medium:
		return 25
	default:
		return 200
	}
}

// decodeString renders the literal content of a string node. Interpolated
// expressions (f-string fields, template substitutions, PHP variables) are
// replaced by a {name} slot so the analyzer sees them as injection surface,
// and returned raw in slots.
func decodeString(lang *language, root *sitter.Node, src []byte) (string, []string) {
	var b strings.Builder
	var slots []string
	var visit func(n *sitter.Node)
	visit = func(n *sitter.Node) {
		kind := n.Kind()
		switch {
		case lang.literalKinds[kind]:
			if kind == "escape_sequence" {
				b.WriteString(unescape(n.Utf8Text(src)))
				return
			}
			// Literal content may still contain named escape children.
			if n.NamedChildCount() == 0 {
				b.WriteString(n.Utf8Text(src))
				return
			}
			writeContentWithEscapes(&b, n, src)
		case lang.delimiterKinds[kind]:
			// skip quotes and heredoc markers
		case n.Id() != root.Id() && n.IsNamed() &&
			!lang.stringRoots[kind] && !lang.containerKinds[kind]:
			// Anything else named inside a string literal is an interpolation.
			expr := n.Utf8Text(src)
			slots = append(slots, expr)
			b.WriteString("{" + slotName(expr, len(slots)) + "}")
		default:
			for i := uint(0); i < n.NamedChildCount(); i++ {
				visit(n.NamedChild(i))
			}
		}
	}
	visit(root)
	if b.Len() == 0 && len(slots) == 0 {
		// Plain quoted string with anonymous content (no named children):
		// fall back to the raw span without its delimiters.
		return stripDelimiters(root.Utf8Text(src)), nil
	}
	return b.String(), slots
}

// writeContentWithEscapes writes a content node whose escape sequences are
// named children interleaved with raw text.
func writeContentWithEscapes(b *strings.Builder, n *sitter.Node, src []byte) {
	pos := n.StartByte()
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		b.Write(src[pos:c.StartByte()])
		if c.Kind() == "escape_sequence" {
			b.WriteString(unescape(c.Utf8Text(src)))
		} else {
			b.Write(src[c.StartByte():c.EndByte()])
		}
		pos = c.EndByte()
	}
	b.Write(src[pos:n.EndByte()])
}

func unescape(seq string) string {
	if len(seq) < 2 || seq[0] != '\\' {
		return seq
	}
	switch seq[1] {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case '\'', '"', '\\', '`', '$', '{':
		return seq[1:]
	default:
		return seq // keep unknown escapes verbatim
	}
}

// slotName derives a readable identifier from an interpolated expression.
func slotName(expr string, idx int) string {
	expr = strings.Trim(expr, "{}$ \t\n")
	var b strings.Builder
	lastUnderscore := false
	for _, r := range expr {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return fmt.Sprintf("value_%d", idx)
	}
	return name
}

func stripDelimiters(s string) string {
	for _, q := range []string{`"""`, "'''", "`", `"`, "'"} {
		if strings.HasPrefix(s, q) && strings.HasSuffix(s, q) && len(s) >= 2*len(q) {
			return s[len(q) : len(s)-len(q)]
		}
	}
	return s
}

// looksLikeProse reports whether a string with no structural evidence still
// looks like a natural-language prompt worth analyzing.
func looksLikeProse(s string) bool {
	if len([]rune(s)) < 200 {
		return false
	}
	return len(strings.Fields(s)) >= 30 && letterRatio(s) >= 0.5
}

// looksLikeInstruction is the lighter gate applied to medium-confidence
// candidates: a few words of mostly letters. It rejects rule specs, ids and
// enum values bound to prompt-like names.
func looksLikeInstruction(s string) bool {
	return len(strings.Fields(s)) >= 4 && letterRatio(s) >= 0.5
}

func letterRatio(s string) float64 {
	letters, total := 0, 0
	for _, r := range s {
		total++
		if isLetter(r) {
			letters++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(letters) / float64(total)
}

func isLetter(r rune) bool {
	return r == ' ' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= 0x00C0
}
