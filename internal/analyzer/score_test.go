package analyzer

import (
	"math"
	"testing"
)

func TestBandFor(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0, "LOW"},
		{4.99, "LOW"},
		{5, "MODERATE"},
		{11.99, "MODERATE"},
		{12, "HIGH"},
		{21.99, "HIGH"},
		{22, "CRITICAL"},
		{100, "CRITICAL"},
	}
	for _, tt := range tests {
		if got := BandFor(tt.score).Label; got != tt.want {
			t.Errorf("BandFor(%g) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

func TestGuardrailReliefIsCapped(t *testing.T) {
	// Many guardrails must never relieve more than maxReliefPercent of the
	// raw score, and never push the score below zero.
	m := &Metrics{Decisions: 10, Constraints: 1000}
	got := branchingScore(m)
	raw := 10.0 * weightDecision
	wantMin := raw * (1 - maxReliefPercent)
	if got < wantMin-0.001 || got > raw {
		t.Errorf("branchingScore = %g, want within [%g, %g]", got, wantMin, raw)
	}
}

func TestScoreNeverNegative(t *testing.T) {
	m := &Metrics{Constraints: 50}
	if got := branchingScore(m); got < 0 {
		t.Errorf("branchingScore = %g, want >= 0", got)
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
