package promptfile

import (
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		path    string
		kind    Kind
		conf    Confidence
		needsFM bool
	}{
		{".claude/skills/deploy/SKILL.md", KindSkill, High, false},
		{"plugins/foo/skills/bar/SKILL.md", KindSkill, High, false},
		{"CLAUDE.md", KindRules, High, false},
		{"packages/api/CLAUDE.md", KindRules, High, false},
		{"CLAUDE.local.md", KindRules, High, false},
		{"AGENTS.md", KindRules, High, false},
		{"GEMINI.md", KindRules, High, false},
		{".cursorrules", KindRules, High, false},
		{".cursor/rules/backend.mdc", KindRules, High, false},
		{".github/copilot-instructions.md", KindRules, High, false},
		{".github/instructions/tests.instructions.md", KindRules, High, false},
		{".github/agents/reviewer.agent.md", KindAgent, High, false},
		{".clinerules/general.md", KindRules, High, false},
		{".windsurf/rules/style.md", KindRules, High, false},
		{".kiro/steering/product.md", KindRules, High, false},
		{".claude/agents/reviewer.md", KindAgent, High, false},
		{".claude/agents/review/security.md", KindAgent, High, false},
		{"plugins/foo/agents/reviewer.md", KindAgent, High, true},
		{".claude/commands/release.md", KindCommand, High, false},
		{"commands/deploy.md", KindCommand, High, true},
		{"prompts/system.md", KindTemplate, Medium, false},
		{"app/prompt_templates/summary.jinja", KindTemplate, Medium, false},
		{"app/summarize.prompt", KindTemplate, Medium, false},
		{"app/summarize.prompt.md", KindTemplate, Medium, false},
		{"docs/system_prompt.txt", KindTemplate, Low, false},
	}
	for _, c := range cases {
		m, ok := Detect(c.path)
		if !ok {
			t.Errorf("Detect(%q) = no match, want %s", c.path, c.kind)
			continue
		}
		if m.Kind != c.kind || m.Confidence != c.conf || m.RequireFrontmatter != c.needsFM {
			t.Errorf("Detect(%q) = %s/%d/fm=%v, want %s/%d/fm=%v",
				c.path, m.Kind, m.Confidence, m.RequireFrontmatter, c.kind, c.conf, c.needsFM)
		}
	}

	notPrompts := []string{
		"README.md", "docs/guide.md", "CHANGELOG.md", "notes.txt",
		"docs/providers/gemini.md", // documentation about Gemini, lower-case
		"docs/llms/claude.md",
		"src/main.py", "templates/email.twig", "docs/prompting.md",
		"prompt.txt.bak",
	}
	for _, p := range notPrompts {
		if m, ok := Detect(p); ok {
			t.Errorf("Detect(%q) = %s, want no match", p, m.Kind)
		}
	}
}

func TestIsTemplatePath(t *testing.T) {
	cases := []struct {
		s              string
		isPath, strong bool
	}{
		{"prompts/system.md", true, false},
		{"summary.jinja", true, true},
		{"templates/answer.prompt", true, true},
		{"notes.txt", true, false},
		{"https://example.com/x.md", false, false},
		{"Read the file prompts/system.md carefully", false, false},
		{"{name}.md", false, false},
		{"main.py", false, false},
	}
	for _, c := range cases {
		isPath, strong := IsTemplatePath(c.s)
		if isPath != c.isPath || strong != c.strong {
			t.Errorf("IsTemplatePath(%q) = %v/%v, want %v/%v", c.s, isPath, strong, c.isPath, c.strong)
		}
	}
}

const skillDoc = `---
name: deploy
description: >
  Deploys the application.
  Use when the user asks to ship or release.
allowed-tools: Bash
---

# Deploy

If the tree is dirty, stop.
`

func TestParseFrontmatter(t *testing.T) {
	m, _ := Detect(".claude/skills/deploy/SKILL.md")
	f := Parse(".claude/skills/deploy/SKILL.md", []byte(skillDoc), m)
	if !f.HasFrontmatter || f.FrontmatterError != "" {
		t.Fatalf("frontmatter not parsed: %+v", f)
	}
	if f.Name != "deploy" {
		t.Errorf("Name = %q", f.Name)
	}
	if !strings.HasPrefix(f.Description, "Deploys the application.") {
		t.Errorf("Description = %q", f.Description)
	}
	if strings.Contains(f.Body, "allowed-tools") || !strings.HasPrefix(strings.TrimSpace(f.Body), "# Deploy") {
		t.Errorf("Body should be the text after the frontmatter, got %q", f.Body)
	}
	if f.BodyLine != 8 {
		t.Errorf("BodyLine = %d, want 8", f.BodyLine)
	}
	if f.Lines != 12 {
		t.Errorf("Lines = %d, want 12", f.Lines)
	}
	if !Accept(f, m) {
		t.Error("skill should be accepted")
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	f := Parse("CLAUDE.md", []byte("# Rules\n\nAlways run tests.\n"), Match{Kind: KindRules})
	if f.HasFrontmatter || f.BodyLine != 1 || f.Kind != KindRules {
		t.Errorf("unexpected parse: %+v", f)
	}
	f = Parse("prompt.txt", []byte("Say hello."), Match{})
	if f.Kind != KindPrompt || f.Context != "prompt file" {
		t.Errorf("explicit file should default to KindPrompt, got %+v", f)
	}
}

func TestParseInvalidFrontmatter(t *testing.T) {
	f := Parse("SKILL.md", []byte("---\nname: [broken\n---\nbody text here\n"), Match{Kind: KindSkill})
	if !f.HasFrontmatter || f.FrontmatterError == "" {
		t.Fatalf("expected a frontmatter error, got %+v", f)
	}
	hints := Hints(f, nil)
	if !hasRule(hints, "frontmatter-invalid") {
		t.Errorf("expected frontmatter-invalid hint, got %+v", hints)
	}
}

func TestAcceptRequiresAgentFrontmatter(t *testing.T) {
	m, _ := Detect("docs/agents/overview.md")
	if !m.RequireFrontmatter {
		t.Fatal("generic agents/ directory should require frontmatter")
	}
	docusaurus := "---\ntitle: Agents\nsidebar_position: 2\n---\n\nAgents are great, read on.\n"
	if Accept(Parse("docs/agents/overview.md", []byte(docusaurus), m), m) {
		t.Error("a docs page with title/sidebar frontmatter is not a subagent")
	}
	agent := "---\nname: reviewer\ndescription: Reviews code.\n---\n\nYou review pull requests carefully.\n"
	if !Accept(Parse("plugins/x/agents/reviewer.md", []byte(agent), m), m) {
		t.Error("a file with name and description is a subagent")
	}
	if Accept(Parse("plugins/x/agents/empty.md", []byte("---\nname: x\ndescription: y\n---\n"), m), m) {
		t.Error("an empty body is not a prompt")
	}
}

func TestParseContentInfersKind(t *testing.T) {
	if f := ParseContent(skillDoc); f.Kind != KindSkill {
		t.Errorf("skill frontmatter → %s", f.Kind)
	}
	agent := "---\nname: r\ndescription: d\ntools: Read, Grep\n---\nbody words here now\n"
	if f := ParseContent(agent); f.Kind != KindAgent {
		t.Errorf("tools frontmatter → %s", f.Kind)
	}
	if f := ParseContent("Just a prompt."); f.Kind != KindPrompt {
		t.Errorf("plain text → %s", f.Kind)
	}
}

func TestSkillHints(t *testing.T) {
	m, _ := Detect("SKILL.md")

	noTrigger := "---\nname: deploy\ndescription: Deploys the application.\n---\n\nRun the deploy script now.\n"
	hints := Hints(Parse("SKILL.md", []byte(noTrigger), m), nil)
	if !hasRule(hints, "description-without-trigger") {
		t.Errorf("expected description-without-trigger, got %+v", hints)
	}

	withTrigger := "---\ndescription: Deploys the app. Use when the user asks to ship.\n---\n\nRun the deploy script now.\n"
	if hints := Hints(Parse("SKILL.md", []byte(withTrigger), m), nil); len(hints) != 0 {
		t.Errorf("a good skill should have no hints, got %+v", hints)
	}

	noDesc := "---\nname: deploy\n---\n\nRun the deploy script now.\n"
	if hints := Hints(Parse("SKILL.md", []byte(noDesc), m), nil); !hasRule(hints, "missing-description") {
		t.Errorf("expected missing-description, got %+v", hints)
	}

	noFM := "# Deploy\n\nRun the deploy script now.\n"
	if hints := Hints(Parse("SKILL.md", []byte(noFM), m), nil); !hasRule(hints, "missing-frontmatter") {
		t.Errorf("expected missing-frontmatter, got %+v", hints)
	}

	long := withTrigger + strings.Repeat("- one more instruction line\n", 600)
	if hints := Hints(Parse("SKILL.md", []byte(long), m), nil); !hasRule(hints, "body-too-long") {
		t.Errorf("expected body-too-long, got %+v", hints)
	}
}

func TestAgentHints(t *testing.T) {
	m, _ := Detect(".claude/agents/reviewer.md")
	bad := "---\nname: Code Reviewer\n---\n\nYou review code with care.\n"
	hints := Hints(Parse(".claude/agents/reviewer.md", []byte(bad), m), nil)
	if !hasRule(hints, "invalid-name") || !hasRule(hints, "missing-description") {
		t.Errorf("expected invalid-name and missing-description, got %+v", hints)
	}
}

func TestRulesHints(t *testing.T) {
	m, _ := Detect("CLAUDE.md")
	long := strings.Repeat("- always do the thing\n", 400)
	hints := Hints(Parse("CLAUDE.md", []byte(long), m), nil)
	if !hasRule(hints, "body-too-long") {
		t.Errorf("expected body-too-long on a 400-line CLAUDE.md, got %+v", hints)
	}
	if h := hintFor(hints, "body-too-long"); h.Severity != "info" {
		t.Errorf("rules length hint should be info, got %s", h.Severity)
	}
}

func TestBrokenReferences(t *testing.T) {
	m, _ := Detect("SKILL.md")
	body := `---
description: Use when deploying.
---

Read [the checklist](references/checklist.md) and run ` + "`scripts/deploy.sh`" + `.
Also see ` + "`scripts/present.sh`" + ` and [docs](https://example.com/x.md).
Examples like ` + "`src/index.ts`" + ` are fine, so are placeholders ` + "`agents/[name].md`" + `.
@docs/style.md
`
	files := map[string]bool{"scripts": true, "scripts/present.sh": true}
	stat := func(rel string) (bool, bool) {
		if !files[rel] {
			return false, false
		}
		return true, rel == "scripts"
	}
	hints := Hints(Parse("SKILL.md", []byte(body), m), stat)
	var refs []string
	for _, h := range hints {
		if h.Rule == "broken-reference" {
			refs = append(refs, h.Message)
		}
	}
	want := []string{"references/checklist.md", "scripts/deploy.sh", "docs/style.md"}
	if len(refs) != len(want) {
		t.Fatalf("got %d broken references, want %d: %v", len(refs), len(want), refs)
	}
	for i, w := range want {
		if !strings.Contains(refs[i], w) {
			t.Errorf("hint %d = %q, want mention of %s", i, refs[i], w)
		}
	}
	if h := hintFor(hints, "broken-reference"); h.Line != 5 {
		t.Errorf("first broken reference should be on line 5, got %d", h.Line)
	}

	// Without a filesystem, reference checks are skipped entirely.
	if hints := Hints(Parse("SKILL.md", []byte(body), m), nil); hasRule(hints, "broken-reference") {
		t.Error("no stat, no reference hints")
	}
}

func hasRule(hints []Hint, rule string) bool {
	for _, h := range hints {
		if h.Rule == rule {
			return true
		}
	}
	return false
}

func hintFor(hints []Hint, rule string) Hint {
	for _, h := range hints {
		if h.Rule == rule {
			return h
		}
	}
	return Hint{}
}
