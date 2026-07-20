package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/extractor"
	"github.com/halleck45/promptcc/internal/report"
)

const scanUsage = `promptcc scan finds and analyzes prompts inside source code.

Usage:
  promptcc scan [flags] <path> [<path>...]

Supported languages: Python, TypeScript, PHP.

Flags:
  --json                    output JSON instead of text
  --full                    print the full report for each prompt
  --min-confidence LEVEL    low, medium or high (default low)
  --fail-over SCORE         exit with code 1 if any prompt scores above SCORE
`

type scanResult struct {
	File       string           `json:"file"`
	Line       int              `json:"line"`
	EndLine    int              `json:"end_line"`
	Confidence string           `json:"confidence"`
	Context    string           `json:"context"`
	Slots      []string         `json:"slots,omitempty"`
	Metrics    analyzer.Metrics `json:"metrics"`
}

func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptcc scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, scanUsage) }
	jsonOut := fs.Bool("json", false, "output JSON")
	full := fs.Bool("full", false, "full report per prompt")
	failOver := fs.Float64("fail-over", -1, "exit 1 if any score exceeds this value")
	minConfidence := fs.String("min-confidence", "low", "low, medium or high")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) == 0 {
		fs.Usage()
		return 2
	}
	minConf, err := extractor.ParseConfidence(*minConfidence)
	if err != nil {
		fmt.Fprintf(stderr, "promptcc: %v\n", err)
		return 2
	}

	prompts, err := extractor.Scan(fs.Args(), extractor.Options{MinConfidence: minConf})
	if err != nil {
		fmt.Fprintf(stderr, "promptcc: %v\n", err)
		return 2
	}

	results := make([]scanResult, 0, len(prompts))
	for _, p := range prompts {
		name := fmt.Sprintf("%s:%d", p.File, p.Line)
		m := analyzer.Analyze(p.Text, name)
		results = append(results, scanResult{
			File:       p.File,
			Line:       p.Line,
			EndLine:    p.EndLine,
			Confidence: p.Confidence.String(),
			Context:    p.Context,
			Slots:      p.Slots,
			Metrics:    m,
		})
	}

	if *jsonOut {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		renderScan(stdout, results, *full)
	}

	if *failOver >= 0 {
		for _, r := range results {
			if r.Metrics.BranchingScore > *failOver {
				fmt.Fprintf(stderr, "promptcc: %s scores %g, above threshold %g\n",
					r.Metrics.Name, r.Metrics.BranchingScore, *failOver)
				return 1
			}
		}
	}
	return 0
}

func renderScan(w io.Writer, results []scanResult, full bool) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No prompts found.")
		return
	}
	for _, r := range results {
		if full {
			fmt.Fprintf(w, "%s  [%s]  %s\n", r.Metrics.Name, r.Confidence, r.Context)
			fmt.Fprintln(w, report.Text(r.Metrics))
			continue
		}
		band := analyzer.BandFor(r.Metrics.BranchingScore)
		fmt.Fprintf(w, "%s  [%s]  %s\n", r.Metrics.Name, r.Confidence, r.Context)
		fmt.Fprintf(w, "    score %g [%s]  decisions=%d density=%g routes=%d inject=%d guards=%d\n",
			r.Metrics.BranchingScore, band.Label, r.Metrics.Decisions,
			r.Metrics.DecisionRatio, r.Metrics.ToolRoutes,
			r.Metrics.InjectionChannels, r.Metrics.Constraints)
	}

	files := map[string]bool{}
	for _, r := range results {
		files[r.File] = true
	}
	fmt.Fprintf(w, "\n%d prompt(s) in %d file(s)\n", len(results), len(files))

	sorted := make([]scanResult, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Metrics.BranchingScore > sorted[j].Metrics.BranchingScore
	})
	fmt.Fprintln(w, "\n── worst offenders "+repeatRune('─', 26))
	for i, r := range sorted {
		if i >= 10 {
			break
		}
		band := analyzer.BandFor(r.Metrics.BranchingScore)
		fmt.Fprintf(w, "  %8.2f  %-9s %s\n", r.Metrics.BranchingScore, band.Label, r.Metrics.Name)
	}
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
