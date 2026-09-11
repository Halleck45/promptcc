package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempPrompt(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const branchyPrompt = `If the user asks for a refund, use the refund tool.
When the user is angry, escalate to a human.
Unless resolved, always follow up. Never guess.`

func TestRunFile(t *testing.T) {
	path := writeTempPrompt(t, branchyPrompt)
	var stdout, stderr bytes.Buffer
	code := run([]string{path}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Branching score") {
		t.Errorf("missing report in output:\n%s", stdout.String())
	}
}

func TestRunStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-"}, strings.NewReader(branchyPrompt), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "stdin") {
		t.Errorf("stdin input should be named 'stdin':\n%s", stdout.String())
	}
}

func TestRunJSON(t *testing.T) {
	path := writeTempPrompt(t, branchyPrompt)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--json", path}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded %d results, want 1", len(decoded))
	}
}

func TestRunFailOver(t *testing.T) {
	path := writeTempPrompt(t, branchyPrompt)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--fail-over", "0.1", path}, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 when above threshold", code)
	}
	if code := run([]string{"--fail-over", "1000", path}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0 when below threshold", code)
	}
}

func TestRunMultipleFilesShowsComparison(t *testing.T) {
	p1 := writeTempPrompt(t, "Say hello.")
	p2 := writeTempPrompt(t, branchyPrompt)
	var stdout, stderr bytes.Buffer
	code := run([]string{p1, p2}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "comparison") {
		t.Errorf("missing comparison table:\n%s", stdout.String())
	}
}

func TestRunMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"/nonexistent/prompt.txt"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2 for missing file", code)
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "promptcc") {
		t.Errorf("version output = %q", stdout.String())
	}
}

func TestRunScanTestdata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--json", filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var decoded []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) < 5 {
		t.Errorf("decoded %d prompts, want at least 5 across the three fixtures", len(decoded))
	}
}

func TestRunScanFailOver(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--fail-over", "0.1", filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 above threshold", code)
	}
}

func TestRunSourceFileGoesThroughExtractor(t *testing.T) {
	// A supported source file must be scanned for prompts, not analyzed as
	// one big prompt text.
	var stdout, stderr bytes.Buffer
	code := run([]string{filepath.Join("..", "..", "internal", "extractor", "testdata", "sample.py")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "prompt(s) in") {
		t.Errorf("source file should produce scan output:\n%s", stdout.String())
	}
}

func TestRunRemovedScanSubcommandStillWorks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"scan", filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "removed") {
		t.Errorf("expected a removal notice on stderr, got: %s", stderr.String())
	}
}

func TestRunScanDefaultIsSummaryOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	out := stdout.String()
	if strings.Contains(out, "decisions=") {
		t.Errorf("default output should not contain per-prompt detail:\n%s", out)
	}
	if !strings.Contains(out, "worst offenders") {
		t.Errorf("default output should contain the summary table:\n%s", out)
	}
}

func TestRunScanVerbose(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--verbose", filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "decisions=") {
		t.Errorf("verbose output should contain per-prompt detail:\n%s", stdout.String())
	}
}

func TestRunScanHTMLReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.html")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--report-html", path, filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("HTML report not written: %v", err)
	}
	html := string(b)
	if !strings.Contains(html, "<!doctype html>") || !strings.Contains(html, "sample.py") {
		t.Errorf("HTML report looks wrong (%d bytes)", len(b))
	}
}

const skillDoc = `---
name: deploy
description: Deploys the application.
---

# Deploy

Read ` + "`references/checklist.md`" + ` first.

## Preflight

If the tree is dirty, stop. Unless the user insists, never force push.

## Reporting

Always report the version.
`

func writeSkill(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".claude", "skills", "deploy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skillDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunSkillFileShowsHintsAndSections(t *testing.T) {
	path := writeSkill(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{path}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Description: Deploys the application.",
		"Hotspot sections",
		"Preflight",
		"Hints:",
		"references/checklist.md",
		"not when to use it",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRunScanDirectoryWithSkill(t *testing.T) {
	path := writeSkill(t)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path))))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--verbose", root}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "[high]  skill deploy") {
		t.Errorf("verbose line should name the skill:\n%s", out)
	}
	if !strings.Contains(out, "1 prompt(s) in 1 file(s): 1 skill") {
		t.Errorf("summary should count kinds:\n%s", out)
	}
	if !strings.Contains(out, "warn: references references/checklist.md") {
		t.Errorf("verbose output should list hints:\n%s", out)
	}
}

func TestRunJSONIncludesKindAndSections(t *testing.T) {
	path := writeSkill(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--json", path}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var decoded []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded %d entries, want 1", len(decoded))
	}
	e := decoded[0]
	if e["kind"] != "skill" || e["name"] != "deploy" {
		t.Errorf("kind/name = %v/%v", e["kind"], e["name"])
	}
	if secs, _ := e["sections"].([]any); len(secs) != 3 {
		t.Errorf("sections = %v, want 3 (blank preamble is skipped)", e["sections"])
	}
	if hints, _ := e["hints"].([]any); len(hints) != 2 {
		t.Errorf("hints = %v, want 2", e["hints"])
	}
	if _, ok := e["metrics"].(map[string]any); !ok {
		t.Errorf("metrics missing: %v", e)
	}
}

func TestRunStdinWithFrontmatter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-"}, strings.NewReader(skillDoc), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Description: Deploys the application.") {
		t.Errorf("stdin frontmatter should be parsed:\n%s", stdout.String())
	}
}
