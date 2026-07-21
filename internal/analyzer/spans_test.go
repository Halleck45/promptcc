package analyzer

import (
	"math"
	"testing"
)

func spanTexts(text string, spans [][2]int) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, text[s[0]:s[1]])
	}
	return out
}

func TestDecisionSpans(t *testing.T) {
	text := "If overdue, escalate. Sauf si le client conteste."
	got := spanTexts(text, DecisionSpans(text))
	want := []string{"If", "Sauf si"}
	if len(got) != len(want) {
		t.Fatalf("DecisionSpans = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecisionSpansUnicodeOffsets(t *testing.T) {
	// Typographic apostrophe and accented words shift byte offsets; the
	// spans must still point at the original text.
	text := "Réponds brièvement. Lorsqu’une erreur survient, préviens l’équipe."
	got := spanTexts(text, DecisionSpans(text))
	if len(got) != 1 || got[0] != "Lorsqu’une" {
		t.Errorf("DecisionSpans = %v, want [Lorsqu’une]", got)
	}
}

func TestDecisionSpansEmpty(t *testing.T) {
	if got := DecisionSpans("Say hello politely."); len(got) != 0 {
		t.Errorf("DecisionSpans = %v, want none", got)
	}
}

func TestBreakdownMatchesScore(t *testing.T) {
	m := Analyze(
		`If A, use the search tool. When B, escalate. Always answer in JSON {"a": {"b": 1}}. Never guess.`,
		"t")
	parts, relief, raw := Breakdown(&m)
	sum := 0.0
	for _, p := range parts {
		sum += p.Value
	}
	if math.Abs(sum-raw) > 1e-9 {
		t.Errorf("sum of components = %g, want raw %g", sum, raw)
	}
	want := math.Round(math.Max(0, raw-relief)*100) / 100
	if m.BranchingScore != want {
		t.Errorf("BranchingScore = %g, want %g from Breakdown", m.BranchingScore, want)
	}
}

func TestBandScaleMatchesBandFor(t *testing.T) {
	for _, b := range BandScale() {
		if got := BandFor(b.Min); got.Label != b.Label {
			t.Errorf("BandFor(%g) = %s, want %s", b.Min, got.Label, b.Label)
		}
	}
}
