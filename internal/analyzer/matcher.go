package analyzer

// The matcher works on word tokens rather than raw regular expressions. This
// keeps matching Unicode-correct (regexp \b is ASCII-only and breaks on
// accented French words) and makes lexicon entries readable and contributable.

// maxGap is the maximum number of tokens a "*" wildcard may skip.
const maxGap = 3

// phrase is a lexicon entry compiled to tokens. A "*" element matches a gap
// of 0 to maxGap arbitrary tokens, e.g. "ne * jamais".
type phrase struct {
	raw    string
	tokens []string
}

// compilePhrases turns raw lexicon entries into phrases, longest first so
// that "si et seulement si" wins over "si" during category-level matching.
func compilePhrases(entries []string) []phrase {
	out := make([]phrase, 0, len(entries))
	for _, e := range entries {
		// tokenize drops "*" (not a word rune), so split on spaces first and
		// tokenize each field, keeping wildcards intact.
		var toks []string
		for _, field := range splitFields(e) {
			if field == "*" {
				toks = append(toks, "*")
				continue
			}
			toks = append(toks, tokenize(field)...)
		}
		if len(toks) > 0 {
			out = append(out, phrase{raw: e, tokens: toks})
		}
	}
	sortByLengthDesc(out)
	return out
}

func splitFields(s string) []string {
	var fields []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				fields = append(fields, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		fields = append(fields, cur)
	}
	return fields
}

func sortByLengthDesc(ps []phrase) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && len(ps[j].tokens) > len(ps[j-1].tokens); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

// matchAt tries to match p starting at tokens[i], honoring the consumed mask
// for literal tokens. It returns the literal token positions on success.
func matchAt(tokens []string, consumed []bool, i int, p phrase) []int {
	var used []int
	pos := i
	for ti := 0; ti < len(p.tokens); ti++ {
		want := p.tokens[ti]
		if want == "*" {
			if ti+1 >= len(p.tokens) {
				return used // trailing wildcard is a no-op
			}
			next := p.tokens[ti+1]
			for skip := 0; skip <= maxGap; skip++ {
				j := pos + skip
				if j < len(tokens) && !consumed[j] && tokens[j] == next {
					pos = j
					goto matchedNext
				}
			}
			return nil
		matchedNext:
			continue
		}
		if pos >= len(tokens) || consumed[pos] || tokens[pos] != want {
			return nil
		}
		used = append(used, pos)
		pos++
	}
	return used
}

// countMatches counts non-overlapping occurrences of each phrase in tokens.
// Phrases within one category share a consumed mask, longest first, so that
// "ne réponds jamais" is not also counted as a bare "jamais".
func countMatches(tokens []string, phrases []phrase) (int, map[string]int) {
	consumed := make([]bool, len(tokens))
	perPhrase := map[string]int{}
	total := 0
	for _, p := range phrases {
		for i := 0; i < len(tokens); i++ {
			if consumed[i] || tokens[i] != firstLiteral(p) {
				continue
			}
			used := matchAt(tokens, consumed, i, p)
			if used == nil {
				continue
			}
			for _, u := range used {
				consumed[u] = true
			}
			perPhrase[p.raw]++
			total++
		}
	}
	return total, perPhrase
}

// containsAny reports whether tokens contain at least one phrase occurrence.
func containsAny(tokens []string, phrases []phrase) bool {
	consumed := make([]bool, len(tokens))
	for _, p := range phrases {
		for i := 0; i < len(tokens); i++ {
			if tokens[i] != firstLiteral(p) {
				continue
			}
			if matchAt(tokens, consumed, i, p) != nil {
				return true
			}
		}
	}
	return false
}

func firstLiteral(p phrase) string {
	for _, t := range p.tokens {
		if t != "*" {
			return t
		}
	}
	return ""
}
