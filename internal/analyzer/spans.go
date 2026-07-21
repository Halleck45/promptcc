package analyzer

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// tokenSpan is a word token with its byte offsets in the original text.
type tokenSpan struct {
	value      string
	start, end int
}

// tokenizeSpans mirrors tokenize but keeps byte offsets into text, so that
// matched tokens can be located back in the source for highlighting.
func tokenizeSpans(text string) []tokenSpan {
	var spans []tokenSpan
	var runes []rune
	var offs [][2]int
	flush := func() {
		lo, hi := 0, len(runes)
		for lo < hi && (runes[lo] == '\'' || runes[lo] == '-') {
			lo++
		}
		for hi > lo && (runes[hi-1] == '\'' || runes[hi-1] == '-') {
			hi--
		}
		if lo < hi {
			spans = append(spans, tokenSpan{string(runes[lo:hi]), offs[lo][0], offs[hi-1][1]})
		}
		runes = runes[:0]
		offs = offs[:0]
	}
	for i, r := range text {
		n := unicode.ToLower(r)
		if n == '’' {
			n = '\''
		}
		if isTokenRune(n) {
			runes = append(runes, n)
			offs = append(offs, [2]int{i, i + utf8.RuneLen(r)})
		} else {
			flush()
		}
	}
	flush()
	return spans
}

// DecisionSpans returns the byte-offset ranges of decision keyword
// occurrences in text, ascending and non-overlapping. Matching mirrors
// countMatches (longest phrase first over a shared consumed mask), so the
// highlighted occurrences are exactly the ones the score counted.
// Consecutive matched tokens (multi-word phrases) merge into one range.
func DecisionSpans(text string) [][2]int {
	toks := tokenizeSpans(text)
	words := make([]string, len(toks))
	for i, t := range toks {
		words[i] = t.value
	}
	consumed := make([]bool, len(words))
	var used []int
	for _, p := range decisionPhrases {
		for i := range words {
			if consumed[i] || words[i] != firstLiteral(p) {
				continue
			}
			m := matchAt(words, consumed, i, p)
			if m == nil {
				continue
			}
			for _, u := range m {
				consumed[u] = true
			}
			used = append(used, m...)
		}
	}
	sort.Ints(used)
	var out [][2]int
	for k := 0; k < len(used); {
		j := k
		for j+1 < len(used) && used[j+1] == used[j]+1 {
			j++
		}
		out = append(out, [2]int{toks[used[k]].start, toks[used[j]].end})
		k = j + 1
	}
	return out
}
