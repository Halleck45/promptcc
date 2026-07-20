package analyzer

import (
	"strings"
	"unicode"
)

// isTokenRune reports whether r belongs inside a word token. Apostrophes and
// hyphens are kept so that elisions ("l'outil") and compounds ("comporte-toi")
// stay single tokens and can be matched as written in the lexicon.
func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-'
}

// tokenize splits text into lowercase word tokens. Typographic apostrophes
// are normalized to ASCII so lexicon entries only need one spelling.
func tokenize(text string) []string {
	text = strings.ReplaceAll(text, "’", "'")
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := strings.Trim(b.String(), "'-")
		if tok != "" {
			tokens = append(tokens, tok)
		}
		b.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if isTokenRune(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
