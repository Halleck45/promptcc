package analyzer

import (
	"regexp"
	"strings"
)

// Injection surface: the channels through which surrounding code pushes
// values into the prompt. Each channel is a distinct coupling point between
// code and prompt.
//
// Patterns are applied in order and each match is masked before the next
// pattern runs, so "{{name}}" is never also counted as "{name}", and
// "${name}" is never also counted as "$name".

type injectionPattern struct {
	re    *regexp.Regexp
	label string
}

var injectionPatterns = []injectionPattern{
	// Template engines allow dotted paths and filters: {{ user.name | upper }}.
	{regexp.MustCompile(`\{\{\s*[a-zA-Z_][\w.]*(?:\s*\|[^{}]*)?\s*\}\}`), "template {{name}}"},
	{regexp.MustCompile(`\[\[\s*[a-zA-Z_]\w*\s*\]\]`), "double bracket [[name]]"},
	{regexp.MustCompile(`%\([a-zA-Z_]\w*\)[sdif]`), "printf %(name)s"},
	// Both spellings of shell interpolation are one and the same channel.
	{regexp.MustCompile(`\$\{[a-zA-Z_]\w*\}`), "shell $name"},
	{regexp.MustCompile(`\$[a-zA-Z_]\w*`), "shell $name"},
	{regexp.MustCompile(`\{[a-zA-Z_]\w*\}`), "brace {name}"},
}

// placeholderTagRe matches "<name>" style placeholders. A match only counts
// as an injection slot when no matching closing tag "</name>" exists in the
// text: paired tags are structural markup (XML sections are common in
// prompts), not value slots.
var placeholderTagRe = regexp.MustCompile(`<([a-zA-Z_][\w-]*)>`)

// countInjections returns the number of distinct channels, the total number
// of slots, and the per-channel detail.
func countInjections(text string) (channels, slots int, detail map[string]int) {
	detail = map[string]int{}
	masked := text
	for _, p := range injectionPatterns {
		hits := p.re.FindAllStringIndex(masked, -1)
		if len(hits) == 0 {
			continue
		}
		detail[p.label] += len(hits)
		b := []byte(masked)
		for _, h := range hits {
			for i := h[0]; i < h[1]; i++ {
				b[i] = ' '
			}
		}
		masked = string(b)
	}

	tagSlots := 0
	for _, m := range placeholderTagRe.FindAllStringSubmatch(masked, -1) {
		name := m[1]
		if strings.Contains(text, "</"+name+">") {
			continue
		}
		tagSlots++
	}
	if tagSlots > 0 {
		detail["placeholder <name>"] = tagSlots
	}

	for _, n := range detail {
		channels++
		slots += n
	}
	return channels, slots, detail
}

// maxBraceDepth returns the maximum nesting depth of braces and brackets,
// a proxy for the depth of the requested output schema.
func maxBraceDepth(text string) int {
	depth, maxDepth := 0, 0
	for _, r := range text {
		switch r {
		case '{', '[':
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return maxDepth
}
