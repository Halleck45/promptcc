// Package report renders analysis results for humans (text) and machines (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/halleck45/promptcc/internal/analyzer"
)

// JSON renders results as an indented JSON array.
func JSON(results []analyzer.Metrics) (string, error) {
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Text renders one result as a human-readable report.
func Text(m analyzer.Metrics) string {
	return header(m) + body(m)
}

// EntryText renders a scan entry: the score, then what a prompt file adds
// (description, hotspot sections, hints) since that is what the reader acts
// on, then the metric detail.
func EntryText(e ScanEntry) string {
	return header(e.Metrics) + basis(e) + Extras(e) + body(e.Metrics)
}

// basis explains a section-based verdict on one line.
func basis(e ScanEntry) string {
	if e.ScoreBasis == "" {
		return ""
	}
	out := fmt.Sprintf("  Judged by its hottest section: %s (line %d, %d sections)\n",
		truncate(e.ScoreBasis, 60), e.Line-1+e.BasisLine, len(e.Sections))
	if d := e.Document; d != nil {
		out += fmt.Sprintf("  Whole file: %d decisions, %d guardrails, %d words\n", d.Decisions, d.Constraints, d.Words)
	}
	return out
}

func header(m analyzer.Metrics) string {
	band := analyzer.BandFor(m.BranchingScore)
	rule := strings.Repeat("─", max(1, 44-len([]rune(m.Name))))
	return fmt.Sprintf("── %s %s\n  Branching score: %g  [%s]  %s\n",
		m.Name, rule, m.BranchingScore, band.Label, band.Hint)
}

func body(m analyzer.Metrics) string {
	var b strings.Builder
	w := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}
	w("")
	w("  What matters (distinct things):")
	w("    decision points          n_decisions   = %d", m.Decisions)
	w("    decision density         p_dec_ratio   = %g", m.DecisionRatio)
	w("    routing / escalation     n_tool_routes = %d", m.ToolRoutes)
	w("    injection channels       inject_surf   = %d  (%d slots)", m.InjectionChannels, m.InjectionSlots)
	w("    output schema depth      output_depth  = %d", m.OutputDepth)
	w("    explicit guardrails      n_constraints = %d  (more guardrails, easier maintenance)", m.Constraints)
	w("    role definitions         n_roles       = %d", m.Roles)
	w("")
	w("  Volume (control group, predicts nothing):")
	w("    %d words · %d chars · %d instruction units (%s)",
		m.Words, m.Chars, m.Instructions, m.Detail.InstructionUnitKind)

	if len(m.Detail.DecisionsByKeyword) > 0 {
		w("")
		w("  Decision keywords:")
		for _, kv := range sortedByCount(m.Detail.DecisionsByKeyword) {
			w("    %3d× %s", kv.N, kv.K)
		}
	}
	if len(m.Detail.InjectionByChannel) > 0 {
		w("")
		w("  Injection channels:")
		for _, kv := range sortedByCount(m.Detail.InjectionByChannel) {
			w("    %3d× %s", kv.N, kv.K)
		}
	}
	return b.String()
}

// maxHotspots caps the hotspot sections listed in text reports.
const maxHotspots = 5

// Extras renders what a prompt file adds to the metrics report: its
// frontmatter description, the sections that carry the branching, and lint
// hints. It returns "" for prompts embedded in code with no sections.
func Extras(e ScanEntry) string {
	var b strings.Builder
	w := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}
	if e.Description != "" {
		w("")
		w("  Description: %s", truncate(e.Description, 160))
	}
	if hot := analyzer.Hotspots(e.Sections, maxHotspots); len(hot) > 0 {
		w("")
		w("  Hotspot sections (%d sections):", len(e.Sections))
		for _, s := range hot {
			band := analyzer.BandFor(s.Metrics.BranchingScore)
			w("    %6.2f  %-9s %s  (line %d, %d decisions)",
				s.Metrics.BranchingScore, band.Label, truncate(s.Heading, 50),
				e.Line-1+s.Line, s.Metrics.Decisions)
			if a := analyzer.Advice(s.Metrics, e.Kind); a != "" {
				w("                     → %s", a)
			}
		}
	}
	if len(e.Hints) > 0 {
		w("")
		w("  Hints:")
		for _, h := range e.Hints {
			loc := ""
			if h.Line > 0 {
				loc = fmt.Sprintf(" (line %d)", h.Line)
			}
			w("    %-4s %s%s", h.Severity, h.Message, loc)
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Comparison renders a score-sorted summary table for multiple results.
func Comparison(results []analyzer.Metrics) string {
	sorted := make([]analyzer.Metrics, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].BranchingScore > sorted[j].BranchingScore
	})
	var b strings.Builder
	b.WriteString("── comparison " + strings.Repeat("─", 31) + "\n")
	for _, m := range sorted {
		band := analyzer.BandFor(m.BranchingScore)
		fmt.Fprintf(&b, "  %8.2f  %-9s %s\n", m.BranchingScore, band.Label, m.Name)
	}
	return b.String()
}

type kv struct {
	K string
	N int
}

func sortedByCount(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, n := range m {
		out = append(out, kv{k, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].K < out[j].K
	})
	return out
}
