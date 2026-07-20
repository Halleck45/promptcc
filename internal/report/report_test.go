package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/halleck45/promptcc/internal/analyzer"
)

func TestJSONRoundTrip(t *testing.T) {
	m := analyzer.Analyze("If asked, use the search tool. Never guess.", "p1")
	out, err := JSON([]analyzer.Metrics{m})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []analyzer.Metrics
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Name != "p1" {
		t.Fatalf("decoded = %+v", decoded)
	}
	if decoded[0].BranchingScore != m.BranchingScore {
		t.Errorf("score lost in round trip: %g != %g", decoded[0].BranchingScore, m.BranchingScore)
	}
	for _, field := range []string{"n_decisions", "p_dec_ratio", "inject_surf", "branching_score"} {
		if !strings.Contains(out, field) {
			t.Errorf("JSON output missing field %q", field)
		}
	}
}

func TestTextContainsKeyMetrics(t *testing.T) {
	m := analyzer.Analyze("If asked about {topic}, use the search tool.", "my-prompt")
	out := Text(m)
	for _, want := range []string{"my-prompt", "Branching score", "n_decisions", "inject_surf", "control group"} {
		if !strings.Contains(out, want) {
			t.Errorf("Text output missing %q\n%s", want, out)
		}
	}
}

func TestComparisonSortsByScoreDescending(t *testing.T) {
	low := analyzer.Analyze("Say hello.", "low")
	high := analyzer.Analyze("If A do X. If B do Y. If C escalate. Unless D, route to E.", "high")
	out := Comparison([]analyzer.Metrics{low, high})
	if strings.Index(out, "high") > strings.Index(out, "low") {
		t.Errorf("comparison not sorted by score desc:\n%s", out)
	}
}
