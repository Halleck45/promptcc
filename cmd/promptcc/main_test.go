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
	code := run([]string{"scan", "--json", filepath.Join("..", "..", "internal", "extractor", "testdata")},
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
	code := run([]string{"scan", "--fail-over", "0.1", filepath.Join("..", "..", "internal", "extractor", "testdata")},
		strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 above threshold", code)
	}
}

func TestRunScanNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"scan"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", code)
	}
}
