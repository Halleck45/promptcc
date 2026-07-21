package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/report"
)

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
		fmt.Fprintln(w, "\nUse --verbose for per-prompt detail, or --report-html report.html.")
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
