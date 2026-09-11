// Package promptfile recognizes prompt files: Markdown and text files that
// carry instructions for a model outside of any source code.
//
// Two families are covered. Agent instruction files are the configuration
// of AI coding assistants (CLAUDE.md, AGENTS.md, Claude Code skills,
// subagents and commands, Cursor and Copilot rules, ...): the whole file is
// a prompt, loaded verbatim into the model's context. Prompt templates
// (.prompt files, prompts/ directories, Jinja templates loaded by code) are
// prompts that were moved out of string literals into their own file.
//
// The package is pure Go with no filesystem access of its own, so it also
// serves the WebAssembly playground.
package promptfile

import (
	"path"
	"regexp"
	"strings"
)

// Kind classifies a prompt file by the role it plays.
type Kind string

const (
	// KindSkill is an Agent Skills SKILL.md (Claude Code, Codex, ...).
	KindSkill Kind = "skill"
	// KindAgent is a subagent definition (.claude/agents/*.md).
	KindAgent Kind = "agent"
	// KindCommand is a slash command (.claude/commands/*.md).
	KindCommand Kind = "command"
	// KindRules is an always-loaded instruction file: CLAUDE.md, AGENTS.md,
	// .cursorrules, copilot-instructions.md, ...
	KindRules Kind = "rules"
	// KindTemplate is a prompt template loaded by application code.
	KindTemplate Kind = "template"
	// KindPrompt is a plain prompt file passed explicitly on the command
	// line, with no other evidence about its role.
	KindPrompt Kind = "prompt"
)

// Confidence mirrors the extractor's scale: 0 low, 1 medium, 2 high. It is
// defined here so the package stays free of cgo dependencies.
type Confidence int

const (
	Low Confidence = iota
	Medium
	High
)

// Match is the verdict of Detect for one path.
type Match struct {
	Kind       Kind
	Context    string // human label, e.g. "Claude Code skill"
	Confidence Confidence
	// RequireFrontmatter marks locations that are only prompt files when the
	// content carries YAML frontmatter: a generic agents/*.md directory holds
	// subagents in a plugin and prose everywhere else.
	RequireFrontmatter bool
}

// templateExts are extensions of files that hold text with placeholders.
var templateExts = map[string]bool{
	".md": true, ".txt": true, ".text": true, ".markdown": true,
	".jinja": true, ".j2": true, ".jinja2": true, ".tpl": true, ".tmpl": true,
	".hbs": true, ".handlebars": true, ".mustache": true, ".njk": true,
	".twig": true, ".liquid": true,
}

// strongTemplateExts are extensions that, on their own, identify a file as a
// prompt template even without a prompt-like name or location.
var strongTemplateExts = map[string]bool{
	".prompt": true, ".prompty": true,
}

// promptDirRe matches directory names that conventionally hold prompt
// templates: prompts, prompt_templates, system_prompts, ...
var promptDirRe = regexp.MustCompile(`(?i)^(system[_-]?)?prompts?([_-]?templates?)?$`)

// promptNameRe matches file names that conventionally hold a prompt.
var promptNameRe = regexp.MustCompile(`(?i)prompt|system[_-]?message|instructions`)

// Detect classifies a path as a prompt file from its name and location
// alone. It returns false for paths that are not prompt files; callers
// still need to Parse the content and may then reject it (see
// Match.RequireFrontmatter).
func Detect(p string) (Match, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	segs := strings.Split(strings.Trim(p, "/"), "/")
	n := len(segs)
	rawBase := segs[n-1]
	base := strings.ToLower(rawBase)
	ext := strings.ToLower(path.Ext(base))
	dir := func(up int) string {
		if n-1-up < 0 {
			return ""
		}
		return strings.ToLower(segs[n-1-up])
	}
	certain := func(k Kind, ctx string) (Match, bool) {
		return Match{Kind: k, Context: ctx, Confidence: High}, true
	}

	// Agent Skills: SKILL.md at the root of a skill directory, wherever it
	// lives (.claude/skills, ~/.claude/skills, plugin skills/, .agents/skills).
	if base == "skill.md" {
		return certain(KindSkill, "skill "+dir(1))
	}

	// Instruction files are conventionally upper-case; a lower-case
	// docs/providers/gemini.md is documentation about Gemini, not for it.
	switch rawBase {
	case "CLAUDE.md", "CLAUDE.local.md":
		return certain(KindRules, "Claude Code instructions")
	case "AGENTS.md":
		return certain(KindRules, "AGENTS.md instructions")
	case "GEMINI.md":
		return certain(KindRules, "Gemini CLI instructions")
	}
	switch base {
	case ".cursorrules":
		return certain(KindRules, "Cursor rules")
	case ".windsurfrules":
		return certain(KindRules, "Windsurf rules")
	case ".clinerules":
		return certain(KindRules, "Cline rules")
	case "copilot-instructions.md":
		if dir(1) == ".github" {
			return certain(KindRules, "Copilot instructions")
		}
	}

	isMarkdown := ext == ".md" || ext == ".mdc"
	switch {
	case dir(2) == ".github" && dir(1) == "instructions" && strings.HasSuffix(base, ".instructions.md"):
		return certain(KindRules, "Copilot instructions")
	case dir(2) == ".github" && dir(1) == "agents" && strings.HasSuffix(base, ".agent.md"):
		return certain(KindAgent, "Copilot agent")
	case dir(2) == ".cursor" && dir(1) == "rules" && isMarkdown:
		return certain(KindRules, "Cursor rules")
	case dir(2) == ".claude" && dir(1) == "rules" && isMarkdown:
		return certain(KindRules, "Claude Code rules")
	case dir(2) == ".claude" && dir(1) == "output-styles" && isMarkdown:
		return certain(KindRules, "Claude Code output style")
	case dir(2) == ".windsurf" && dir(1) == "rules" && isMarkdown:
		return certain(KindRules, "Windsurf rules")
	case dir(2) == ".augment" && dir(1) == "rules" && isMarkdown:
		return certain(KindRules, "Augment rules")
	case dir(2) == ".trae" && dir(1) == "rules" && isMarkdown:
		return certain(KindRules, "Trae rules")
	case dir(1) == ".clinerules" && isMarkdown:
		return certain(KindRules, "Cline rules")
	case dir(2) == ".roo" && strings.HasPrefix(dir(1), "rules") && isMarkdown:
		return certain(KindRules, "Roo rules")
	case dir(2) == ".kiro" && dir(1) == "steering" && isMarkdown:
		return certain(KindRules, "Kiro steering")
	case dir(1) == ".junie" && base == "guidelines.md":
		return certain(KindRules, "Junie guidelines")
	case dir(2) == ".gemini" && dir(1) == "commands" && (ext == ".toml" || isMarkdown):
		return certain(KindCommand, "Gemini CLI command")
	}

	// Claude Code subagents and commands: certain under .claude/, otherwise
	// (plugin directories, nested namespaces) only with frontmatter.
	if isMarkdown {
		for up := 1; up <= 3; up++ {
			switch dir(up) {
			case "agents":
				m := Match{Kind: KindAgent, Context: "Claude Code subagent", Confidence: High}
				m.RequireFrontmatter = dir(up+1) != ".claude"
				return m, true
			case "commands":
				m := Match{Kind: KindCommand, Context: "Claude Code command", Confidence: High}
				m.RequireFrontmatter = dir(up+1) != ".claude"
				return m, true
			}
			if strings.HasPrefix(dir(up), ".") {
				break // do not climb out of a dotted config directory
			}
		}
	}

	// Prompt templates.
	if strongTemplateExts[ext] || strings.Contains(base, ".prompt.") {
		return Match{Kind: KindTemplate, Context: "prompt template", Confidence: Medium}, true
	}
	if !templateExts[ext] {
		return Match{}, false
	}
	for up := 1; up <= 3; up++ {
		if d := dir(up); d != "" && promptDirRe.MatchString(d) {
			return Match{Kind: KindTemplate, Context: "in " + segs[n-1-up] + "/", Confidence: Medium}, true
		}
	}
	if promptNameRe.MatchString(strings.TrimSuffix(base, ext)) && base != "prompting.md" {
		return Match{Kind: KindTemplate, Context: "prompt-like file name", Confidence: Low}, true
	}
	return Match{}, false
}

// IsTemplatePath reports whether a string literal found in source code looks
// like a relative path to a prompt template file, and whether its extension
// alone is strong evidence (a .prompt or .jinja file) rather than a generic
// text file (.md, .txt) that needs a prompt-like binding to count.
func IsTemplatePath(s string) (isPath, strong bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 200 || strings.ContainsAny(s, " \n\t\"'<>{}|*") {
		return false, false
	}
	if strings.Contains(s, "://") {
		return false, false
	}
	ext := strings.ToLower(path.Ext(s))
	if strongTemplateExts[ext] || strings.Contains(strings.ToLower(path.Base(s)), ".prompt.") {
		return true, true
	}
	if !templateExts[ext] {
		return false, false
	}
	switch ext {
	case ".md", ".txt", ".text", ".markdown":
		return true, false
	}
	return true, true
}
