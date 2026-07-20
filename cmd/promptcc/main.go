// Command promptcc measures the branching complexity of LLM prompts.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/halleck45/promptcc/internal/analyzer"
	"github.com/halleck45/promptcc/internal/report"
)

// version is set at build time by goreleaser via ldflags.
var version = "dev"

const usage = `promptcc measures the branching complexity of LLM prompts.

Usage:
  promptcc [flags] <file> [<file>...]
  promptcc [flags] -            read the prompt from stdin
  cat prompt.txt | promptcc     stdin is used when piped
  promptcc scan <path>          extract and analyze prompts from source code

Flags:
  --json             output JSON instead of text
  --fail-over SCORE  exit with code 1 if any prompt scores above SCORE
  --version          print version and exit

Run "promptcc scan" with no argument for scan-specific flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "scan" {
		return runScan(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("promptcc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	jsonOut := fs.Bool("json", false, "output JSON")
	failOver := fs.Float64("fail-over", -1, "exit 1 if any score exceeds this value")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "promptcc %s\n", version)
		return 0
	}

	files := fs.Args()
	if len(files) == 0 {
		if stdinIsPiped() {
			files = []string{"-"}
		} else {
			fs.Usage()
			return 2
		}
	}

	var results []analyzer.Metrics
	for _, f := range files {
		text, name, err := readInput(f, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		results = append(results, analyzer.Analyze(text, name))
	}

	if *jsonOut {
		out, err := report.JSON(results)
		if err != nil {
			fmt.Fprintf(stderr, "promptcc: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, out)
	} else {
		for _, m := range results {
			fmt.Fprintln(stdout, report.Text(m))
		}
		if len(results) > 1 {
			fmt.Fprint(stdout, report.Comparison(results))
		}
	}

	if *failOver >= 0 {
		for _, m := range results {
			if m.BranchingScore > *failOver {
				fmt.Fprintf(stderr, "promptcc: %s scores %g, above threshold %g\n",
					m.Name, m.BranchingScore, *failOver)
				return 1
			}
		}
	}
	return 0
}

func readInput(path string, stdin io.Reader) (text, name string, err error) {
	if path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(b), "stdin", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return string(b), path, nil
}

func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
