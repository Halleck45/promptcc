package promptfile

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Hint is an actionable finding about a prompt file that the branching score
// cannot express: a skill Claude will never trigger, a link to a reference
// file that does not exist, a header the loader cannot parse.
type Hint struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // "warn" or "info"
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
}

const (
	// maxSkillLines is the documented ceiling for a SKILL.md body: its
	// content is loaded whole every time the skill triggers.
	maxSkillLines = 500
	// maxRulesLines is where an always-loaded instruction file (CLAUDE.md,
	// .cursorrules) starts to cost more than it guides: every line is paid
	// on every turn of every session.
	maxRulesLines = 300
	// maxDescriptionChars is where a description starts to be truncated in
	// skill listings (the documented cap is 1536 characters including the
	// when_to_use field).
	maxDescriptionChars = 1024
	// maxReferenceHints bounds broken-reference noise per file.
	maxReferenceHints = 5
)

// triggerRe matches wording that tells the model when to use a skill, as
// opposed to only what it does.
var triggerRe = regexp.MustCompile(`(?i)\b(use (this|it|when|whenever|for|to)|when (the |a )?(user|you|asked|working|writing|building|creating|debugging)|whenever|trigger|invoke|for (any|all|every|tasks|requests|questions)|helps? (you|with)|if (the |a )?user|utilise|quand|lorsque)\b`)

// referenceRe finds relative file references in markdown links and inline
// code, and CLAUDE.md "@path" imports at the start of a line.
var referenceRe = regexp.MustCompile("(?m)\\]\\(([^)\\s]+)\\)|`([^`\\s]+)`|^@([^\\s]+)")

// Stat resolves a path relative to a prompt file's directory. It reports
// whether something exists there and whether it is a directory.
type Stat func(rel string) (exists, isDir bool)

// Hints lints a parsed file. stat resolves paths relative to the file's
// directory; pass nil when no filesystem is available and reference checks
// are skipped.
func Hints(f *File, stat Stat) []Hint {
	var out []Hint
	add := func(rule, severity, msg string, line int) {
		out = append(out, Hint{Rule: rule, Severity: severity, Message: msg, Line: line})
	}

	if f.FrontmatterError != "" {
		add("frontmatter-invalid", "warn", "frontmatter is not valid YAML: "+f.FrontmatterError, 1)
	}

	switch f.Kind {
	case KindSkill:
		if f.HasFrontmatter && f.FrontmatterError == "" {
			if f.Description == "" {
				add("missing-description", "warn",
					"no description in the frontmatter: the description is what the model reads to decide when to use this skill (it falls back to the first line of the body)", 1)
			} else {
				if !triggerRe.MatchString(f.Description) && scalar(f.Frontmatter, "when_to_use") == "" {
					add("description-without-trigger", "info",
						"the description says what the skill does but not when to use it; add a \"Use when ...\" clause", 1)
				}
				if n := len([]rune(f.Description)); n > maxDescriptionChars {
					add("description-too-long", "info",
						fmt.Sprintf("description is %d characters; skill listings truncate long descriptions", n), 1)
				}
			}
		} else if !f.HasFrontmatter {
			add("missing-frontmatter", "warn",
				"no YAML frontmatter: without a description, the model only sees the first line of the body to decide when to use this skill", 1)
		}
		if f.Lines > maxSkillLines {
			add("body-too-long", "warn",
				fmt.Sprintf("%d lines; keep SKILL.md under %d lines and move reference material to separate files the skill points to", f.Lines, maxSkillLines), 0)
		}
	case KindAgent:
		if f.HasFrontmatter && f.FrontmatterError == "" {
			if f.Name == "" {
				add("missing-name", "warn", "subagents need a name in the frontmatter", 1)
			} else if strings.ContainsAny(f.Name, ": ") || strings.HasPrefix(f.Name, "-") || f.Name != strings.ToLower(f.Name) {
				add("invalid-name", "warn",
					fmt.Sprintf("subagent name %q should be lowercase letters and hyphens, without a colon", f.Name), 1)
			}
			if f.Description == "" {
				add("missing-description", "warn",
					"subagents need a description: it is what the main agent reads to decide when to delegate", 1)
			}
		} else if !f.HasFrontmatter {
			add("missing-frontmatter", "warn", "subagent files need a YAML frontmatter with name and description", 1)
		}
		if f.Lines > maxSkillLines {
			add("body-too-long", "info",
				fmt.Sprintf("%d lines loaded into the subagent's context on every delegation", f.Lines), 0)
		}
	case KindRules:
		if f.Lines > maxRulesLines {
			add("body-too-long", "info",
				fmt.Sprintf("%d lines loaded into every session; move task-specific guidance into skills or imported files", f.Lines), 0)
		}
	}

	if len(strings.Fields(f.Body)) < 4 && f.Kind != KindPrompt {
		add("empty-body", "warn", "the body carries no instructions", f.BodyLine)
	}

	if stat != nil {
		out = append(out, referenceHints(f, stat)...)
	}
	return out
}

// resourceDirs are the conventional directories a skill ships alongside its
// SKILL.md. A reference into one of them is a promise the model will act on.
var resourceDirs = map[string]bool{"scripts": true, "references": true, "assets": true}

// referenceHints reports relative file references in the body that do not
// resolve on disk. Only explicit references count (markdown links, inline
// code, @imports), and only when they point somewhere the file can vouch
// for: a directory that exists next to it, a skill resource directory, or
// an @import. Everything else ("src/index.ts" in a scaffolding command) is
// an example, not a reference.
func referenceHints(f *File, stat Stat) []Hint {
	var out []Hint
	seen := map[string]bool{}
	for lineNo, line := range strings.Split(f.Body, "\n") {
		for _, m := range referenceRe.FindAllStringSubmatch(line, -1) {
			ref := m[1] + m[2] + m[3]
			isImport := m[3] != ""
			ref = strings.TrimSuffix(strings.TrimPrefix(ref, "./"), "/")
			if !looksLikeRelativeFile(ref) || seen[ref] {
				continue
			}
			seen[ref] = true
			if exists, _ := stat(ref); exists {
				continue
			}
			first := ref[:strings.Index(ref, "/")]
			_, dirExists := stat(first)
			if !isImport && !dirExists && !(f.Kind == KindSkill && resourceDirs[first]) {
				continue
			}
			out = append(out, Hint{
				Rule:     "broken-reference",
				Severity: "warn",
				Message:  fmt.Sprintf("references %s, which does not exist next to this file", ref),
				Line:     f.BodyLine + lineNo,
			})
			if len(out) >= maxReferenceHints {
				return out
			}
		}
	}
	return out
}

// looksLikeRelativeFile keeps references that name a file inside the
// repository: a relative path with a directory component or a recognizable
// extension, and nothing that reads as a URL, a placeholder or a glob.
func looksLikeRelativeFile(ref string) bool {
	if ref == "" || len(ref) > 200 {
		return false
	}
	if strings.ContainsAny(ref, "*?{}<>$#[]()") || strings.Contains(ref, "://") ||
		strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "~") ||
		strings.HasPrefix(ref, "mailto:") || strings.HasPrefix(ref, "..") {
		return false
	}
	ext := path.Ext(ref)
	if !strings.Contains(ref, "/") {
		return false
	}
	// A directory component alone is not enough: "src/foo" could be a
	// module or a route. Require a file extension of a plausible length.
	return len(ext) >= 2 && len(ext) <= 6 && !strings.ContainsAny(ext, " -")
}
