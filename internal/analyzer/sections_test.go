package analyzer

import (
	"strings"
	"testing"
)

const sectionedPrompt = `You are a deployment assistant.

# Preflight

If the tree is dirty, stop. Unless the user insists, never force push.

## Targets

When the change touches billing, escalate to a human.

` + "```" + `
# not a heading, this is a shell comment
if [ -f x ]; then echo hi; fi
` + "```" + `

# Reporting

Always report the version.
`

func TestSections(t *testing.T) {
	secs := Sections(sectionedPrompt)
	if len(secs) != 4 {
		t.Fatalf("got %d sections, want 4 (preamble + 3 headings): %+v", len(secs), headings(secs))
	}
	want := []struct {
		heading string
		level   int
		line    int
	}{
		{"(preamble)", 0, 1},
		{"Preflight", 1, 3},
		{"Targets", 2, 7},
		{"Reporting", 1, 16},
	}
	for i, w := range want {
		if secs[i].Heading != w.heading || secs[i].Level != w.level || secs[i].Line != w.line {
			t.Errorf("section %d = %q/%d/line %d, want %q/%d/line %d",
				i, secs[i].Heading, secs[i].Level, secs[i].Line, w.heading, w.level, w.line)
		}
	}
	if secs[1].Metrics.Decisions != 2 {
		t.Errorf("Preflight decisions = %d, want 2 (if, unless)", secs[1].Metrics.Decisions)
	}
	// The fenced block stays inside Targets (its "# ..." line is not a
	// heading), so its shell "if" is analyzed as part of that section.
	if secs[2].Metrics.Decisions != 2 {
		t.Errorf("Targets decisions = %d, want 2 (prose 'when' + fenced 'if')", secs[2].Metrics.Decisions)
	}
	if secs[3].Metrics.Constraints != 1 {
		t.Errorf("Reporting constraints = %d, want 1", secs[3].Metrics.Constraints)
	}
}

func TestSectionsNeedTwoHeadings(t *testing.T) {
	if s := Sections("# Only one\n\nIf x then y."); s != nil {
		t.Errorf("one heading should yield no sections, got %+v", headings(s))
	}
	if s := Sections("No headings at all. If x then y."); s != nil {
		t.Errorf("no heading should yield no sections, got %+v", headings(s))
	}
}

func TestHotspots(t *testing.T) {
	secs := Sections(sectionedPrompt)
	hot := Hotspots(secs, 2)
	if len(hot) != 2 {
		t.Fatalf("got %d hotspots, want 2", len(hot))
	}
	if hot[0].Metrics.BranchingScore < hot[1].Metrics.BranchingScore {
		t.Error("hotspots must be sorted by score, highest first")
	}
	for _, h := range hot {
		if h.Metrics.BranchingScore == 0 {
			t.Errorf("zero-score section %q should not be a hotspot", h.Heading)
		}
	}
}

func TestTemplateSlotsWithPathsAndFilters(t *testing.T) {
	m := Analyze("Hello {{ user.name | title }}, your order {{order.id}} ships to {{ address }}.", "t")
	if m.InjectionSlots != 3 || m.InjectionChannels != 1 {
		t.Errorf("slots = %d, channels = %d, want 3 slots in 1 channel: %v",
			m.InjectionSlots, m.InjectionChannels, m.Detail.InjectionByChannel)
	}
}

func headings(secs []Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Heading
	}
	return out
}

func TestJudgeUsesHottestSubstantialSection(t *testing.T) {
	text := `# Usage

If asked, run it.

# Rules

- Always run the tests before committing.
- If the tests fail, stop and report.
- When the branch is not main, ask before pushing.
- Unless told otherwise, never force push.

# Notes

Plain prose without any condition at all.
`
	j := Judge(text, "doc")
	if j.Basis != "Rules" {
		t.Fatalf("basis = %q, want Rules (Usage is too small to judge by): %+v", j.Basis, j)
	}
	if j.Document == nil || j.Document.Decisions < j.Metrics.Decisions {
		t.Errorf("document metrics should cover the whole text: %+v", j.Document)
	}
	if j.Metrics.Name != "doc" {
		t.Errorf("judging metrics keep the prompt name, got %q", j.Metrics.Name)
	}
	if j.BasisLine != 5 {
		t.Errorf("BasisLine = %d, want 5", j.BasisLine)
	}

	flat := Judge("If x then y. Otherwise z.", "flat")
	if flat.Basis != "" || flat.Document != nil || flat.Sections != nil {
		t.Errorf("a prompt without sections is judged whole: %+v", flat)
	}
}

func TestAdvice(t *testing.T) {
	low := Metrics{BranchingScore: 3}
	if a := Advice(low, "rules"); a != "" {
		t.Errorf("no advice below HIGH, got %q", a)
	}
	hot := Metrics{BranchingScore: 15, DecisionRatio: 0.3, Constraints: 2}
	if a := Advice(hot, "rules"); !strings.Contains(a, "skill") {
		t.Errorf("rules advice should point to skills, got %q", a)
	}
	if a := Advice(hot, ""); !strings.Contains(a, "route first") {
		t.Errorf("code advice should suggest routing, got %q", a)
	}
	bare := Metrics{BranchingScore: 15, DecisionRatio: 0.6, Constraints: 0, InjectionChannels: 3}
	a := Advice(bare, "skill")
	if !strings.Contains(a, "never / always") || !strings.Contains(a, "3 injection channels") {
		t.Errorf("unexpected advice %q", a)
	}
}
