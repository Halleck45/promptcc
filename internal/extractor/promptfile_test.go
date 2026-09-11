package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halleck45/promptcc/internal/promptfile"
)

// writeTree creates files under root from a path → content map.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func byRelPath(t *testing.T, root string, prompts []Prompt) map[string]Prompt {
	t.Helper()
	out := map[string]Prompt{}
	for _, p := range prompts {
		rel, err := filepath.Rel(root, p.File)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.ToSlash(rel)] = p
	}
	return out
}

func TestScanFindsAgentInstructionFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"CLAUDE.md": "# Rules\n\nAlways run the tests. If they fail, stop.\n",
		".claude/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Ship it.\n---\n\n" +
			"# Deploy\n\nIf the tree is dirty, stop. See `scripts/deploy.sh`.\n",
		".claude/agents/reviewer.md": "---\nname: reviewer\ndescription: Reviews code.\n---\n\nYou review pull requests with care.\n",
		"plugins/x/commands/ship.md": "---\ndescription: Ship a release.\n---\n\nTag the release and push it.\n",
		"docs/agents/overview.md":    "---\ntitle: Agents\n---\n\nAgents are documented here at length.\n",
		"README.md":                  "# Project\n\nIf you want to install, run make. When it fails, retry.\n",
		"docs/providers/gemini.md":   "# Gemini\n\nIf you use Gemini, set the key. When it fails, retry.\n",
	})

	prompts, err := Scan([]string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRelPath(t, root, prompts)

	rules, ok := got["CLAUDE.md"]
	if !ok || rules.Kind != "rules" || rules.Confidence != High || rules.Line != 1 {
		t.Errorf("CLAUDE.md = %+v, want kind rules, high confidence, line 1", rules)
	}

	skill, ok := got[".claude/skills/deploy/SKILL.md"]
	if !ok {
		t.Fatalf("SKILL.md not found in %v", keys(got))
	}
	if skill.Kind != "skill" || skill.Name != "deploy" || skill.Description != "Ship it." {
		t.Errorf("SKILL.md = %+v", skill)
	}
	if skill.Line != 5 || !strings.HasPrefix(strings.TrimSpace(skill.Text), "# Deploy") {
		t.Errorf("SKILL.md body should start right after the closing --- (line 5), got line %d: %q", skill.Line, skill.Text)
	}
	if !hasHint(skill, "description-without-trigger") || !hasHint(skill, "broken-reference") {
		t.Errorf("SKILL.md hints = %+v, want description-without-trigger and broken-reference", skill.Hints)
	}

	if agent, ok := got[".claude/agents/reviewer.md"]; !ok || agent.Kind != "agent" || agent.Name != "reviewer" {
		t.Errorf("subagent = %+v", agent)
	}
	if cmd, ok := got["plugins/x/commands/ship.md"]; !ok || cmd.Kind != "command" {
		t.Errorf("plugin command with frontmatter = %+v", cmd)
	}
	for _, rel := range []string{"docs/agents/overview.md", "README.md", "docs/providers/gemini.md"} {
		if p, ok := got[rel]; ok {
			t.Errorf("%s should not be a prompt, got %+v", rel, p)
		}
	}
}

func TestScanFindsTemplatesAndReferences(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"prompts/summarize.md":       "Summarize {{ document }} in {{ lang }}. If it is a contract, list the parties.\n",
		"prompts/index.twig":         "<div class=\"page\"><p>Hello</p><a href=\"/x\">x</a></div>\n",
		"app/templates/answer.jinja": "You are a helper. If asked about {{ topic | lower }}, answer briefly, unless it is legal advice.\n",
		"app/notes/todo.md":          "Buy milk. If the shop is closed, buy it tomorrow.\n",
		"app/service.py": strings.Join([]string{
			"from pathlib import Path",
			"",
			"SYSTEM_PROMPT_PATH = 'notes/todo.md'",
			"template = Path('templates/answer.jinja').read_text()",
			"readme = open('README.md').read()",
			"",
			"def build():",
			"    return SYSTEM_PROMPT_PATH",
		}, "\n") + "\n",
		"README.md": "# App\n\nIf you install it, it works. When it does not, it does not.\n",
	})

	prompts, err := Scan([]string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRelPath(t, root, prompts)

	if p, ok := got["prompts/summarize.md"]; !ok || p.Kind != "template" || p.Confidence != Medium {
		t.Errorf("prompts/summarize.md = %+v, want a medium-confidence template", p)
	}
	if p, ok := got["prompts/index.twig"]; ok {
		t.Errorf("HTML view under prompts/ should be vetoed, got %+v", p)
	}

	jinja, ok := got["app/templates/answer.jinja"]
	if !ok {
		t.Fatalf("answer.jinja referenced from code not found in %v", keys(got))
	}
	if jinja.Kind != "template" || !strings.HasPrefix(jinja.Context, "referenced from ") || !strings.HasSuffix(jinja.Context, "service.py:4") {
		t.Errorf("answer.jinja = %+v, want context 'referenced from .../service.py:4'", jinja)
	}
	if jinja.Confidence != Medium {
		t.Errorf("answer.jinja without prompt-like binding should be medium, got %d", jinja.Confidence)
	}

	todo, ok := got["app/notes/todo.md"]
	if !ok {
		t.Fatalf("notes/todo.md bound to SYSTEM_PROMPT_PATH not found in %v", keys(got))
	}
	if todo.Confidence != High {
		t.Errorf("a .md bound to a prompt-like name is high confidence, got %d", todo.Confidence)
	}

	if p, ok := got["README.md"]; ok {
		t.Errorf("README.md opened without prompt evidence should not be a prompt, got %+v", p)
	}
	for rel, p := range got {
		if strings.HasSuffix(rel, ".py") {
			t.Errorf("path literals must not be extracted as prompts themselves: %s %+v", rel, p)
		}
	}
}

func TestLoadPromptFileExplicit(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"SKILL.md": "---\ndescription: Use when testing.\n---\n\nDo the thing carefully.\n"})
	p, keep, err := LoadPromptFile(filepath.Join(root, "SKILL.md"), mustDetect(t, "SKILL.md"))
	if err != nil || !keep {
		t.Fatalf("keep = %v, err = %v", keep, err)
	}
	if p.Kind != "skill" || len(p.Hints) != 0 {
		t.Errorf("got %+v", p)
	}
}

func hasHint(p Prompt, rule string) bool {
	for _, h := range p.Hints {
		if h.Rule == rule {
			return true
		}
	}
	return false
}

func keys(m map[string]Prompt) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mustDetect(t *testing.T, path string) promptfile.Match {
	t.Helper()
	m, ok := promptfile.Detect(path)
	if !ok {
		t.Fatalf("Detect(%q) should match", path)
	}
	return m
}

func TestDuplicateRuleHints(t *testing.T) {
	root := t.TempDir()
	rule := "- Always run the full test suite before opening a pull request.\n"
	writeTree(t, root, map[string]string{
		"CLAUDE.md":              "# Rules\n\n" + rule + "- Keep commits small.\n",
		"packages/api/CLAUDE.md": "# API rules\n\n" + rule + "- Use snake_case in SQL.\n",
		"packages/web/CLAUDE.md": "# Web rules\n\nPrefer function components over classes always.\n",
	})
	prompts, err := Scan([]string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRelPath(t, root, prompts)
	for _, rel := range []string{"CLAUDE.md", "packages/api/CLAUDE.md"} {
		if !hasHint(got[rel], "duplicate-rule") {
			t.Errorf("%s should carry a duplicate-rule hint, got %+v", rel, got[rel].Hints)
		}
	}
	if hasHint(got["packages/web/CLAUDE.md"], "duplicate-rule") {
		t.Errorf("web CLAUDE.md has no duplicate, got %+v", got["packages/web/CLAUDE.md"].Hints)
	}
	if h := got["CLAUDE.md"].Hints[0]; h.Line != 3 || !strings.Contains(h.Message, "packages/api/CLAUDE.md:3") {
		t.Errorf("hint should point at the other file and line: %+v", h)
	}
}
