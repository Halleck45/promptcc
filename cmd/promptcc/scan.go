package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/extractor"
	"github.com/halleck45/promptcc/internal/report"
)

const scanUsage = `promptcc scan finds and analyzes prompts inside source code.

Usage:
  promptcc scan [flags] <path> [<path>...]

Supported languages: Python, TypeScript, PHP.

Flags:
  --verbose                 print one line per prompt (default: summary only)
  --full                    print the full text report for each prompt
  --json                    output JSON instead of text
  --html-report FILE        also write a detailed HTML report to FILE
  --min-confidence LEVEL    low, medium or high (default low)
  --fail-over SCORE         exit with code 1 if any prompt scores above SCORE
`

func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptcc scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, scanUsage) }
	jsonOut := fs.Bool("json", false, "output JSON")
	verbose := fs.Bool("verbose", false, "one line per prompt")
	full := fs.Bool("full", false, "full report per prompt")
	htmlReport := fs.String("html-report", "", "write a detailed HTML report to this file")
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

	stopSpinner := startSpinner(!*jsonOut)
	prompts, err := extractor.Scan(fs.Args(), extractor.Options{MinConfidence: minConf})
	if err != nil {
		stopSpinner()
		fmt.Fprintf(stderr, "promptcc: %v\n", err)
		return 2
	}

	entries := make([]report.ScanEntry, 0, len(prompts))
	for _, p := range prompts {
		name := fmt.Sprintf("%s:%d", p.File, p.Line)
		entries = append(entries, report.ScanEntry{
			File:       p.File,
			Line:       p.Line,
			EndLine:    p.EndLine,
			Confidence: p.Confidence.String(),
			Context:    p.Context,
			Slots:      p.Slots,
			Text:       p.Text,
			Metrics:    analyzer.Analyze(p.Text, name),
		})
	}
	stopSpinner()

	if *htmlReport != "" {
		html, err := report.HTML(entries, version)
		if err == nil {
			err = os.WriteFile(*htmlReport, []byte(html), 0o644)
		}
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: writing HTML report: %v\n", err)
			return 2
		}
		fmt.Fprintf(stderr, "HTML report written to %s\n", *htmlReport)
	}

	if *jsonOut {
		b, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		renderScan(stdout, entries, *verbose || *full, *full)
	}

	if *failOver >= 0 {
		for _, e := range entries {
			if e.Metrics.BranchingScore > *failOver {
				fmt.Fprintf(stderr, "promptcc: %s scores %g, above threshold %g\n",
					e.Metrics.Name, e.Metrics.BranchingScore, *failOver)
				return 1
			}
		}
	}
	return 0
}

func renderScan(w io.Writer, entries []report.ScanEntry, verbose, full bool) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No prompts found.")
		return
	}

	if verbose {
		for _, e := range entries {
			fmt.Fprintf(w, "%s  [%s]  %s\n", e.Metrics.Name, e.Confidence, e.Context)
			if full {
				fmt.Fprintln(w, report.Text(e.Metrics))
				continue
			}
			band := analyzer.BandFor(e.Metrics.BranchingScore)
			fmt.Fprintf(w, "    score %g [%s]  decisions=%d density=%g routes=%d inject=%d guards=%d\n",
				e.Metrics.BranchingScore, band.Label, e.Metrics.Decisions,
				e.Metrics.DecisionRatio, e.Metrics.ToolRoutes,
				e.Metrics.InjectionChannels, e.Metrics.Constraints)
		}
		fmt.Fprintln(w)
	}

	files := map[string]bool{}
	for _, e := range entries {
		files[e.File] = true
	}
	fmt.Fprintf(w, "%d prompt(s) in %d file(s)\n", len(entries), len(files))

	sorted := make([]report.ScanEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Metrics.BranchingScore > sorted[j].Metrics.BranchingScore
	})
	fmt.Fprintln(w, "\n── worst offenders "+repeatRune('─', 26))
	for i, e := range sorted {
		if i >= 10 {
			break
		}
		band := analyzer.BandFor(e.Metrics.BranchingScore)
		fmt.Fprintf(w, "  %8.2f  %-9s %s\n", e.Metrics.BranchingScore, band.Label, e.Metrics.Name)
	}
	if !verbose {
		fmt.Fprintln(w, "\nUse --verbose for per-prompt detail, or --html-report report.html.")
	}
}

// startSpinner shows a scanning indicator on stderr while the scan runs.
// It is enabled only when stderr is an interactive terminal, so piped and
// CI output stays clean. The returned function stops it synchronously.
func startSpinner(enabled bool) func() {
	if !enabled || !stderrIsTerminal() {
		return func() {}
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\x1b[K")
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%c scanning...", frames[i%len(frames)])
				i++
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
