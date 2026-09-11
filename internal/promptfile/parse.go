package promptfile

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// File is a parsed prompt file.
type File struct {
	Path       string
	Kind       Kind
	Context    string
	Confidence Confidence

	// Frontmatter holds the YAML header, when present. Name and Description
	// are the two fields every agent-file format shares.
	Frontmatter      map[string]any
	HasFrontmatter   bool
	FrontmatterError string
	Name             string
	Description      string

	// Body is the prompt text without the frontmatter. BodyLine is the
	// 1-based line the body starts on, so that file:line locations point
	// at the prompt rather than at the header. Lines counts the whole file.
	Body     string
	BodyLine int
	Lines    int
}

// Parse builds a File from raw content. The match comes from Detect, or is
// zero for a file passed explicitly with no other evidence.
func Parse(p string, content []byte, m Match) *File {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	f := &File{
		Path:       p,
		Kind:       m.Kind,
		Context:    m.Context,
		Confidence: m.Confidence,
		Body:       text,
		BodyLine:   1,
		Lines:      strings.Count(text, "\n") + 1,
	}
	if f.Kind == "" {
		f.Kind = KindPrompt
		f.Context = "prompt file"
	}
	if fm, body, line, ok := splitFrontmatter(text); ok {
		f.HasFrontmatter = true
		f.Body = body
		f.BodyLine = line
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
			f.FrontmatterError = strings.TrimSpace(strings.TrimPrefix(err.Error(), "yaml: "))
		} else {
			f.Frontmatter = parsed
			f.Name = scalar(parsed, "name")
			f.Description = scalar(parsed, "description")
		}
	}
	return f
}

// ParseContent parses pasted text with no path: the kind is inferred from
// the frontmatter (a tools list means a subagent, any other header a
// skill), so the playground can lint a SKILL.md as the CLI would.
func ParseContent(text string) *File {
	f := Parse("", []byte(text), Match{})
	switch {
	case !f.HasFrontmatter:
		// plain prompt
	case f.Frontmatter["tools"] != nil:
		f.Kind, f.Context = KindAgent, "subagent (from frontmatter)"
	default:
		f.Kind, f.Context = KindSkill, "skill (from frontmatter)"
	}
	return f
}

// agentKeys are frontmatter fields that only agent definitions carry. A
// docs site also has an agents/ section with frontmatter, but its pages
// declare a title and a sidebar position, not a description for a model.
var agentKeys = []string{"description", "allowed-tools", "tools", "argument-hint", "model", "when_to_use"}

// Accept reports whether a parsed file should be kept as a prompt: the
// location may demand an agent-style frontmatter, and the body must carry
// some prose.
func Accept(f *File, m Match) bool {
	if m.RequireFrontmatter {
		if !f.HasFrontmatter {
			return false
		}
		found := false
		for _, k := range agentKeys {
			if _, ok := f.Frontmatter[k]; ok {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return len(strings.Fields(f.Body)) >= 4
}

// splitFrontmatter separates a leading YAML block delimited by "---" lines.
// It returns the raw YAML, the remaining body, and the 1-based line number
// on which the body starts.
func splitFrontmatter(text string) (fm, body string, bodyLine int, ok bool) {
	if !strings.HasPrefix(text, "---\n") && text != "---" {
		return "", "", 0, false
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		t := strings.TrimRight(lines[i], " \t")
		if t == "---" || t == "..." {
			fm = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return fm, body, i + 2, true
		}
	}
	return "", "", 0, false
}

func scalar(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.Join(strings.Fields(yamlString(v)), " "))
	}
}

func yamlString(v any) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
