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
	band := analyzer.BandFor(m.BranchingScore)
	var b strings.Builder
	w := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}

	rule := strings.Repeat("─", max(1, 44-len([]rune(m.Name))))
	w("── %s %s", m.Name, rule)
	w("  Branching score: %g  [%s]  %s", m.BranchingScore, band.Label, band.Hint)
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
