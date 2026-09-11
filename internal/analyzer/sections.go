package analyzer

import (
	"regexp"
	"strings"
)

// Section is one heading-delimited part of a Markdown prompt with its own
// metrics. A 300-line CLAUDE.md rarely branches evenly: sections tell where
// the decision logic actually lives.
type Section struct {
	Heading string  `json:"heading"`
	Level   int     `json:"level"`
	Line    int     `json:"line"` // 1-based, relative to the analyzed text
	Metrics Metrics `json:"metrics"`
}

var (
	headingRe = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t]*#*[ \t]*$`)
	fenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// Sections splits a Markdown prompt on its ATX headings and analyzes each
// part. Headings inside fenced code blocks are ignored. Text before the
// first heading forms a "(preamble)" section when it is not blank. Prompts
// with fewer than two headings return nil: there is nothing to compare.
func Sections(text string) []Section {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	type raw struct {
		heading string
		level   int
		line    int
		start   int // index of the first body line
	}
	var found []raw
	inFence := false
	for i, l := range lines {
		if fenceRe.MatchString(l) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			found = append(found, raw{heading: m[2], level: len(m[1]), line: i + 1, start: i + 1})
		}
	}
	if len(found) < 2 {
		return nil
	}

	var out []Section
	if pre := strings.Join(lines[:found[0].line-1], "\n"); strings.TrimSpace(pre) != "" {
		out = append(out, Section{
			Heading: "(preamble)",
			Level:   0,
			Line:    1,
			Metrics: Analyze(pre, "(preamble)"),
		})
	}
	for i, h := range found {
		end := len(lines)
		if i+1 < len(found) {
			end = found[i+1].line - 1
		}
		body := strings.Join(lines[h.start:end], "\n")
		name := h.heading
		if name == "" {
			name = "(untitled)"
		}
		out = append(out, Section{
			Heading: name,
			Level:   h.level,
			Line:    h.line,
			Metrics: Analyze(body, name),
		})
	}
	return out
}

// Hotspots returns the sections that carry branching, highest score first,
// capped at n. Sections that score zero are left out.
func Hotspots(sections []Section, n int) []Section {
	var out []Section
	for _, s := range sections {
		if s.Metrics.BranchingScore > 0 {
			out = append(out, s)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Metrics.BranchingScore > out[j-1].Metrics.BranchingScore; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Judgment is the analysis of a prompt together with the basis of its
// verdict. A prompt with Markdown sections is judged by its hottest section:
// summing decisions over a 700-line file rewards nothing but length, which
// predicts no maintenance pain, while the section a maintainer actually
// edits is where the branching has to stay consistent.
type Judgment struct {
	// Metrics carries the score and band the prompt is judged by: the
	// hottest section's when the prompt has sections, the whole text's
	// otherwise.
	Metrics Metrics
	// Document holds the whole-text metrics when the verdict came from a
	// section, nil otherwise.
	Document *Metrics
	// Basis is the heading of the judging section, "" for a whole prompt.
	Basis string
	// BasisLine is the 1-based line of that heading in the text.
	BasisLine int
	Sections  []Section
}

// minJudgingUnits is the smallest section that can carry a verdict. A
// two-line section with one "if" has a decision density of 1.0 and would
// outrank every substantial section on that ratio alone.
const minJudgingUnits = 3

// Judge analyzes text and picks the metrics its verdict rests on.
func Judge(text, name string) Judgment {
	j := Judgment{Metrics: Analyze(text, name), Sections: Sections(text)}
	var candidates []Section
	for _, s := range j.Sections {
		if s.Metrics.Instructions >= minJudgingUnits {
			candidates = append(candidates, s)
		}
	}
	hot := Hotspots(candidates, 1)
	if len(hot) == 0 {
		return j
	}
	doc := j.Metrics
	j.Document = &doc
	j.Metrics = hot[0].Metrics
	j.Metrics.Name = name
	j.Basis = hot[0].Heading
	j.BasisLine = hot[0].Line
	return j
}
