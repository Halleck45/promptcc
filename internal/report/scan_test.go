package report

import (
	"strings"
	"testing"

	"github.com/halleck45/promptcc/internal/analyzer"
)

func sampleEntries() []ScanEntry {
	low := analyzer.Analyze("Say hello politely.", "src/a.py:3")
	high := analyzer.Analyze(
		"If A, use tool X. If B, escalate to {team}. Unless C, route to D. Never guess.",
		"src/b.ts:10")
	return []ScanEntry{
		{File: "src/a.py", Line: 3, Confidence: "medium", Context: "var greeting",
			Text: "Say hello politely.", Metrics: low},
		{File: "src/b.ts", Line: 10, Confidence: "high", Context: "call client.messages.create",
			Slots: []string{"${team}"},
			Text:  "If A, use tool X...", Metrics: high},
	}
}

func TestHTMLReport(t *testing.T) {
	out, err := HTML(sampleEntries(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<!doctype html>",
		"src/a.py:3", "src/b.ts:10",
		"decision points", "injection channels", "explicit guardrails",
		"client.messages.create",
		"${team}",
		"1.2.3",
		"Score distribution", "score ≥ 22",
		"Severity breakdown", "Prompt text",
		`id="page-dashboard"`, `id="page-explorer"`, `id="page-help"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML report missing %q", want)
		}
	}
	// Sorted by score: the branchy prompt must appear before the greeting.
	if strings.Index(out, "src/b.ts:10") > strings.Index(out, "src/a.py:3") {
		t.Error("HTML report not sorted by score descending")
	}
}

func TestHTMLReportLowConfidenceToggle(t *testing.T) {
	entries := sampleEntries()
	entries[0].Confidence = "low"
	out, err := HTML(entries, "dev")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="showlow"`,
		"Show 1 low-confidence prompt(s)",
		"lowconf",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML report missing %q", want)
		}
	}
	// Without low-confidence entries the toggle must not render.
	out, err = HTML(sampleEntries(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `id="showlow"`) {
		t.Error("toggle rendered without low-confidence entries")
	}
}

func TestHTMLReportEscapesContent(t *testing.T) {
	entries := []ScanEntry{{
		File: "evil.py", Line: 1, Confidence: "low", Context: "heuristic",
		Text:    `<script>alert("xss")</script> if something, escalate.`,
		Metrics: analyzer.Analyze("x", "evil.py:1"),
	}}
	out, err := HTML(entries, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `<script>alert`) {
		t.Error("prompt text not HTML-escaped")
	}
}

func TestHTMLReportEmpty(t *testing.T) {
	out, err := HTML(nil, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No prompts found.") {
		t.Error("empty report should render")
	}
}

func TestHTMLMedianIgnoresInertFragments(t *testing.T) {
	inert := analyzer.Analyze("Some plain text with nothing branchy at all.", "a:1")
	branchy := analyzer.Analyze("If A, escalate. When B, use tool X. Unless C, route to D.", "b:1")
	entries := []ScanEntry{
		{File: "a.py", Line: 1, Confidence: "medium", Metrics: inert},
		{File: "a.py", Line: 2, Confidence: "medium", Metrics: inert},
		{File: "a.py", Line: 3, Confidence: "medium", Metrics: inert},
		{File: "b.py", Line: 1, Confidence: "medium", Metrics: branchy},
	}
	out, err := HTML(entries, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ">0.00</div><div class=\"text-xs text-[var(--muted)] mt-0.5\">median score") {
		t.Error("median should be computed over scored prompts, not pinned to 0 by inert fragments")
	}
	if !strings.Contains(out, "inert fragments") {
		t.Error("report should surface the inert fragment count")
	}
}
