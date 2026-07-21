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
		if isConcat(lang, n, src) {
			if leaves := flattenConcat(lang, n, src); concatHasString(lang, leaves) {
				if p, ok := w.extractConcat(lang, n, leaves, src, path); ok {
					prompts = append(prompts, p)
				}
				return // the whole expression was consumed as one prompt
			}
			// no string operand (arithmetic): descend normally
		}
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

// isConcat reports whether n is a string concatenation expression.
func isConcat(lang *language, n *sitter.Node, src []byte) bool {
	if lang.concatKind == "" || n.Kind() != lang.concatKind {
		return false
	}
	op := n.ChildByFieldName("operator")
	return op != nil && op.Utf8Text(src) == lang.concatOp
}

// flattenConcat turns a left-nested concatenation tree into its operands,
// in source order.
func flattenConcat(lang *language, n *sitter.Node, src []byte) []*sitter.Node {
	if isConcat(lang, n, src) {
		left, right := n.ChildByFieldName("left"), n.ChildByFieldName("right")
		if left != nil && right != nil {
			return append(flattenConcat(lang, left, src), flattenConcat(lang, right, src)...)
		}
	}
	return []*sitter.Node{n}
}

func concatHasString(lang *language, leaves []*sitter.Node) bool {
	for _, l := range leaves {
		if lang.stringRoots[l.Kind()] {
			return true
		}
	}
	return false
}

// extractConcat merges a concatenation into a single prompt: string operands
// contribute their decoded text, every other operand becomes an injection
// slot ('Respond with JSON, for example: ' . json_encode($example)).
func (w *worker) extractConcat(lang *language, n *sitter.Node, leaves []*sitter.Node, src []byte, path string) (Prompt, bool) {
	if p := n.Parent(); p != nil && p.Kind() == "expression_statement" {
		return Prompt{}, false
	}
	var b strings.Builder
	var slots []string
	for _, leaf := range leaves {
		if lang.stringRoots[leaf.Kind()] {
			text, s := decodeString(lang, leaf, src)
			b.WriteString(text)
			slots = append(slots, s...)
			continue
		}
		expr := leaf.Utf8Text(src)
		slots = append(slots, expr)
		b.WriteString("{" + slotName(expr, len(slots)) + "}")
	}
	return w.finishPrompt(lang, n, src, path, b.String(), slots)
}

// extractString decodes a string literal and classifies it as a prompt.
func (w *worker) extractString(lang *language, n *sitter.Node, src []byte, path string) (Prompt, bool) {
	// A bare string statement does nothing at runtime: it is a docstring
	// (Python) or dead code, never a prompt handed to a model.
	if p := n.Parent(); p != nil && p.Kind() == "expression_statement" {
		return Prompt{}, false
	}
	text, slots := decodeString(lang, n, src)
	return w.finishPrompt(lang, n, src, path, text, slots)
}

// finishPrompt classifies a decoded literal and applies the content gates.
func (w *worker) finishPrompt(lang *language, n *sitter.Node, src []byte, path, text string, slots []string) (Prompt, bool) {
	confidence, context, v := classify(lang, n, src)
	switch v {
	case notPrompt:
		return Prompt{}, false
	case noEvidence:
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
	// SQL reads like prose (CASE WHEN ... THEN ... ELSE) and is full of
	// decision keywords; only strings handed to a known SDK call escape
	// this check. Same for CLI signature DSLs and HTML markup.
	if confidence != High && (looksLikeSQL(text) || looksLikeSpec(text) || looksLikeMarkup(text)) {
		return Prompt{}, false
	}
	// Codegen templates carry AUTO-GENERATED / DO NOT EDIT headers, but so
	// do some prompts ("Do NOT edit existing entries"), so this veto only
	// applies to heuristic hits.
	if confidence == Low && looksLikeGenerated(text) {
		return Prompt{}, false
	}
	// Embedded scripts and source code read like branchy prose (mock
	// servers in tests, hook scripts). These vetoes only apply to heuristic
	// hits: prompts legitimately contain code examples, and prompt-named
	// bindings keep the benefit of the doubt.
	if confidence == Low && (looksLikeScript(text) || looksLikeCode(text)) {
		return Prompt{}, false
	}
	// A single line of hyphenated utility tokens is a CSS class list, even
	// when bound to a Prompt* component name.
	if confidence != High && looksLikeStyleList(text) {
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

// sqlMarkers score SQL-specific constructs. Strong markers (2 points) are
// unambiguous SQL; weak markers (1 point) also occur in instruction prose.
var sqlMarkers = []struct {
	marker string
	weight int
}{
	// unambiguous DML/DDL: a single hit is decisive
	{"insert into", 4}, {"insert or", 4}, {"delete from", 4},
	{"on conflict", 4}, {"create table", 4}, {"alter table", 4},
	{"drop table", 4}, {"create index", 4},
	// strong markers
	{"select ", 2}, {"group by", 2}, {"order by", 2}, {"inner join", 2},
	{"left join", 2}, {"right join", 2}, {"case when", 2}, {"like '%", 2},
	{"union ", 2}, {"having ", 2}, {"sum(", 2}, {"count(", 2}, {"end as", 2},
	{"= ?", 2}, {"= excluded.", 2},
	// weak markers, also common in prose
	{"update ", 1}, {"where ", 1}, {" from ", 1}, {"then ", 1}, {"values (", 1},
	// DDL column definitions (schema fixtures in tests)
	{"primary key", 2}, {"foreign key", 2}, {"not null", 2},
	{"varchar(", 2}, {"autoincrement", 2}, {"auto_increment", 2},
	{"tinyint", 2}, {"references ", 1}, {"default ", 1},
}

// looksLikeSQL reports whether a string is more plausibly a SQL query than
// a prompt. Threshold is 4 points so that prose mentioning a single SQL-ish
// word ("select the best answer from the list") is not rejected.
func looksLikeSQL(s string) bool {
	l := strings.ToLower(s)
	score := 0
	for _, m := range sqlMarkers {
		if strings.Contains(l, m.marker) {
			score += m.weight
			if score >= 4 {
				return true
			}
		}
	}
	return false
}

// looksLikeSpec detects CLI signature DSLs such as Laravel's artisan
// signatures: "{--service=openai : openai or nano_banana}". These are option
// specs, not prompts, however wordy their inline descriptions are.
func looksLikeSpec(s string) bool {
	if strings.Contains(s, "{--") {
		return true
	}
	lines, spec := 0, 0
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		lines++
		// "{name=default : description}" lines; JSON example lines in real
		// prompts contain double quotes and are not counted.
		if strings.HasPrefix(l, "{") && strings.HasSuffix(l, "}") &&
			strings.Contains(l, " : ") && !strings.Contains(l, `"`) {
			spec++
		}
	}
	return lines >= 2 && spec*2 > lines
}

// markupMarkers score HTML-specific constructs. Prompts legitimately use a
// few bare semantic XML tags (<context>, <instructions>); real HTML carries
// attributes, presentational tags and entities, which prompts do not.
var markupMarkers = []struct {
	marker string
	weight int
}{
	{"href=", 2}, {"class=", 2}, {"style=", 2}, {"<script", 2},
	{"<img", 2}, {"<div", 2}, {"<span", 2}, {"</a>", 2},
	{"<p>", 2}, {"</p>", 2}, {"<h1", 2}, {"<h2", 2}, {"<h3", 2},
	{"<br", 1}, {"<li>", 1}, {"<ul>", 1}, {"<strong>", 1}, {"&nbsp;", 1},
	{"<!--", 1},
}

// looksLikeMarkup reports whether a string is more plausibly HTML content
// (templates, translated marketing pages) than a prompt.
func looksLikeMarkup(s string) bool {
	l := strings.ToLower(s)
	score := 0
	for _, m := range markupMarkers {
		if strings.Contains(l, m.marker) {
			score += m.weight
			if score >= 4 {
				return true
			}
		}
	}
	return false
}

// scriptMarkers score shell and PowerShell constructs found in scripts
// embedded as string literals (hook scripts, installers).
var scriptMarkers = []struct {
	marker string
	weight int
}{
	{"#!/", 4},
	{"| out-null", 2}, {"convertto-json", 2}, {"convertfrom-json", 2},
	{"[console]::", 2}, {"invoke-restmethod", 2}, {"invoke-webrequest", 2},
	{"join-path", 2}, {"$env:", 2}, {"} catch", 2}, {"set -e", 2},
	{"process.exit", 2}, {"os.system(", 2}, {"subprocess.", 2},
	{"try {", 1}, {"exit 0", 1}, {"exit 1", 1}, {"esac", 1},
}

// looksLikeScript reports whether a string is more plausibly an embedded
// script than a prompt.
func looksLikeScript(s string) bool {
	l := strings.ToLower(s)
	score := 0
	for _, m := range scriptMarkers {
		if strings.Contains(l, m.marker) {
			score += m.weight
			if score >= 4 {
				return true
			}
		}
	}
	return false
}

// looksLikeCode reports whether a multi-line string is more plausibly
// source code than prose: a large share of lines ending in braces or
// semicolons, or starting like statements and comments.
func looksLikeCode(s string) bool {
	lines, codeLines := 0, 0
	for _, line := range strings.Split(s, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		lines++
		if strings.HasSuffix(l, "{") || strings.HasSuffix(l, "}") ||
			strings.HasSuffix(l, ";") || strings.HasSuffix(l, "});") ||
			strings.HasPrefix(l, "}") || strings.HasPrefix(l, "//") ||
			strings.HasPrefix(l, "#") || strings.HasPrefix(l, "import ") ||
			strings.HasPrefix(l, "from ") || strings.HasPrefix(l, "def ") ||
			strings.HasPrefix(l, "class ") {
			codeLines++
		}
	}
	return lines >= 5 && codeLines*5 >= lines*2 // at least 40% code-shaped lines
}

// generatedMarkers flag scaffolding and codegen templates, which describe
// files for developers, not behavior for a model.
var generatedMarkers = []string{"auto-generated", "autogenerated", "do not edit"}

func looksLikeGenerated(s string) bool {
	l := strings.ToLower(s)
	for _, m := range generatedMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

// looksLikeStyleList reports whether a one-line string is a CSS utility
// class list ("mb-2 px-3 font-medium text-xs").
func looksLikeStyleList(s string) bool {
	if strings.Contains(strings.TrimSpace(s), "\n") {
		return false
	}
	fields := strings.Fields(s)
	if len(fields) < 3 {
		return false
	}
	hyphenated := 0
	for _, f := range fields {
		if strings.Contains(f, "-") || strings.Contains(f, ":") {
			hyphenated++
		}
	}
	return hyphenated*10 >= len(fields)*6 // at least 60% utility-shaped tokens
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
