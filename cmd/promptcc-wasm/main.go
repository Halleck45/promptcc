//go:build js && wasm

// Command promptcc-wasm exposes the analyzer to JavaScript for the static
// web playground (web/). It registers a single global function:
//
//	promptccAnalyze(text) -> JSON string
//
// The payload bundles the metrics, the score breakdown and the band scale so
// the page never duplicates scoring constants.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/halleck45/promptcc/internal/analyzer"
)

type result struct {
	Metrics    analyzer.Metrics     `json:"metrics"`
	Components []analyzer.Component `json:"components"`
	Relief     float64              `json:"relief"`
	Raw        float64              `json:"raw"`
	Band       analyzer.Band        `json:"band"`
	Scale      []analyzer.BandInfo  `json:"scale"`
}

func analyze(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.Null()
	}
	m := analyzer.Analyze(args[0].String(), "pasted prompt")
	parts, relief, raw := analyzer.Breakdown(&m)
	out, err := json.Marshal(result{
		Metrics:    m,
		Components: parts,
		Relief:     relief,
		Raw:        raw,
		Band:       analyzer.BandFor(m.BranchingScore),
		Scale:      analyzer.BandScale(),
	})
	if err != nil {
		return js.Null()
	}
	return string(out)
}

func main() {
	js.Global().Set("promptccAnalyze", js.FuncOf(analyze))
	// Keep the Go runtime alive so the exported function stays callable.
	select {}
}
