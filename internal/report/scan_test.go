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
		"2 prompt(s) in 2 file(s)",
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
	if !strings.Contains(out, "0 prompt(s)") {
		t.Error("empty report should render")
	}
}
