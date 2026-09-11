//go:build js && wasm

// Command promptcc-wasm exposes the analyzer to JavaScript for the static
// web playground (web/). It registers a single global function:
//
//	promptccAnalyze(text) -> JSON string
//
// The payload bundles the metrics, the score breakdown and the band scale so
// the page never duplicates scoring constants. Pasted text is parsed as a
// prompt file first: a YAML frontmatter is stripped and linted the way the
// CLI lints a SKILL.md, and Markdown sections are scored separately.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/promptfile"
)

type fileInfo struct {
	Kind        string            `json:"kind"`
	Context     string            `json:"context"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Frontmatter bool              `json:"frontmatter"`
	BodyLine    int               `json:"body_line"`
	Hints       []promptfile.Hint `json:"hints"`
}

type result struct {
	Metrics    analyzer.Metrics     `json:"metrics"`
	Components []analyzer.Component `json:"components"`
	Relief     float64              `json:"relief"`
	Raw        float64              `json:"raw"`
	Band       analyzer.Band        `json:"band"`
	Scale      []analyzer.BandInfo  `json:"scale"`
	File       fileInfo             `json:"file"`
	Sections   []analyzer.Section   `json:"sections"`
	Advice     []string             `json:"advice"` // one entry per section, "" when none
	ScoreBasis string               `json:"score_basis,omitempty"`
	BasisLine  int                  `json:"score_basis_line,omitempty"`
	Document   *analyzer.Metrics    `json:"document,omitempty"`
}

func analyze(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.Null()
	}
	f := promptfile.ParseContent(args[0].String())
	j := analyzer.Judge(f.Body, "pasted prompt")
	m := j.Metrics
	parts, relief, raw := analyzer.Breakdown(&m)
	advice := make([]string, len(j.Sections))
	for i, sec := range j.Sections {
		advice[i] = analyzer.Advice(sec.Metrics, string(f.Kind))
	}
	out, err := json.Marshal(result{
		Metrics:    m,
		Components: parts,
		Relief:     relief,
		Raw:        raw,
		Band:       analyzer.BandFor(m.BranchingScore),
		Scale:      analyzer.BandScale(),
		File: fileInfo{
			Kind:        string(f.Kind),
			Context:     f.Context,
			Name:        f.Name,
			Description: f.Description,
			Frontmatter: f.HasFrontmatter,
			BodyLine:    f.BodyLine,
			Hints:       promptfile.Hints(f, nil),
		},
		Sections:   j.Sections,
		Advice:     advice,
		ScoreBasis: j.Basis,
		BasisLine:  j.BasisLine,
		Document:   j.Document,
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
