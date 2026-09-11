// Command promptcc measures the branching complexity of LLM prompts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/extractor"
	"github.com/halleck45/promptcc/internal/promptfile"
	"github.com/halleck45/promptcc/internal/report"
)

// version is set at build time by goreleaser via ldflags.
var version = "dev"

const usage = `promptcc measures the branching complexity of LLM prompts.

Usage:
  promptcc [flags] <path> [<path>...]
  cat prompt.txt | promptcc     read the prompt from stdin

Each path may be:
  - a directory: scan its source files (Python, TypeScript, JavaScript, PHP)
    for prompts embedded in code, and pick up prompt files: agent
    instructions (CLAUDE.md, AGENTS.md, SKILL.md, .claude/agents, Cursor and
    Copilot rules, ...) and prompt templates (.prompt, prompts/*.md, files
    referenced from code)
  - a source file: extract and analyze its prompts
  - any other file, or "-": analyze the whole content as one prompt
    (YAML frontmatter is parsed, Markdown sections are scored separately
    and the prompt is judged by its hottest section)

Flags:
  --json                  output JSON instead of text
  --verbose               scan output: one line per prompt (default: summary)
  --full                  print the full text report for each prompt
  --report-html FILE      also write a detailed HTML report to FILE
  --min-confidence LEVEL  scan: low, medium or high (default low)
  --fail-over SCORE       exit with code 1 if any prompt scores above SCORE
  --version               print version and exit
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("promptcc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	jsonOut := fs.Bool("json", false, "output JSON")
	verbose := fs.Bool("verbose", false, "one line per prompt")
	full := fs.Bool("full", false, "full report per prompt")
	reportHTML := fs.String("report-html", "", "write a detailed HTML report to this file")
	minConfidence := fs.String("min-confidence", "low", "low, medium or high")
	failOver := fs.Float64("fail-over", -1, "exit 1 if any score exceeds this value")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "promptcc %s\n", version)
		return 0
	}

	paths := fs.Args()
	// The scan subcommand was folded into the default invocation.
	if len(paths) > 0 && paths[0] == "scan" {
		if _, err := os.Stat("scan"); err != nil {
			fmt.Fprintln(stderr, "promptcc: the scan subcommand was removed, pass paths directly")
			paths = paths[1:]
		}
	}
	if len(paths) == 0 {
		if stdinIsPiped() {
			paths = []string{"-"}
		} else {
			fs.Usage()
			return 2
		}
	}

	minConf, err := extractor.ParseConfidence(*minConfidence)
	if err != nil {
		fmt.Fprintf(stderr, "promptcc: %v\n", err)
		return 2
	}

	// Route each path: directories and supported source files go through the
	// extractor; anything else is analyzed as raw prompt text.
	var scanPaths, textPaths []string
	for _, p := range paths {
		if p == "-" {
			textPaths = append(textPaths, p)
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		if info.IsDir() || extractor.Supports(p) {
			scanPaths = append(scanPaths, p)
		} else {
			textPaths = append(textPaths, p)
		}
	}

	var entries []report.ScanEntry

	stopSpinner := startSpinner(!*jsonOut && len(scanPaths) > 0)
	if len(scanPaths) > 0 {
		prompts, err := extractor.Scan(scanPaths, extractor.Options{MinConfidence: minConf})
		if err != nil {
			stopSpinner()
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		for _, p := range prompts {
			entries = append(entries, entryFor(p, fmt.Sprintf("%s:%d", p.File, p.Line)))
		}
	}
	stopSpinner()

	for _, f := range textPaths {
		content, name, err := readInput(f, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		// An explicit file is always analyzed whole. Its location still
		// tells what it is (a SKILL.md, a CLAUDE.md), which drives the hints.
		m, _ := promptfile.Detect(name)
		e := entryFor(extractor.PromptFromFile(promptfile.Parse(name, content, m)), name)
		e.Confidence = "file"
		entries = append(entries, e)
	}

	if *reportHTML != "" {
		html, err := report.HTML(entries, version)
		if err == nil {
			err = os.WriteFile(*reportHTML, []byte(html), 0o644)
		}
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: writing HTML report: %v\n", err)
			return 2
		}
		fmt.Fprintf(stderr, "HTML report written to %s\n", *reportHTML)
	}

	scanMode := len(scanPaths) > 0
	switch {
	case *jsonOut:
		b, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(b))
	case scanMode:
		renderScan(stdout, entries, *verbose || *full, *full)
	default:
		var all []analyzer.Metrics
		for _, e := range entries {
			fmt.Fprintln(stdout, report.EntryText(e))
			all = append(all, e.Metrics)
		}
		if len(all) > 1 {
			fmt.Fprint(stdout, report.Comparison(all))
		}
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

// entryFor analyzes a prompt and wraps it as a report entry, scoring its
// Markdown sections separately when it has any.
func entryFor(p extractor.Prompt, name string) report.ScanEntry {
	j := analyzer.Judge(p.Text, name)
	return report.ScanEntry{
		File:        p.File,
		Line:        p.Line,
		EndLine:     p.EndLine,
		Confidence:  p.Confidence.String(),
		Context:     p.Context,
		Slots:       p.Slots,
		Text:        p.Text,
		Kind:        p.Kind,
		Name:        p.Name,
		Description: p.Description,
		Hints:       p.Hints,
		Metrics:     j.Metrics,
		ScoreBasis:  j.Basis,
		BasisLine:   j.BasisLine,
		Document:    j.Document,
		Sections:    j.Sections,
	}
}

func readInput(path string, stdin io.Reader) (content []byte, name string, err error) {
	if path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, "", fmt.Errorf("reading stdin: %w", err)
		}
		return b, "stdin", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return b, path, nil
}

func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
