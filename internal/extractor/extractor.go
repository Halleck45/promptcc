// Package extractor finds LLM prompts inside source code.
//
// It parses files with tree-sitter, collects string literals, and classifies
// each one by walking up the syntax tree: a literal passed to a known LLM SDK
// call is a prompt with high confidence, a literal bound to a prompt-like
// name (variable, keyword argument, object key) is medium confidence, and a
// long natural-language literal with no other evidence is low confidence.
package extractor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/halleck45/promptcc/internal/promptfile"
)

// Confidence expresses how sure we are that a string literal is a prompt.
type Confidence int

const (
	Low Confidence = iota
	Medium
	High
)

func (c Confidence) String() string {
	switch c {
	case High:
		return "high"
	case Medium:
		return "medium"
	default:
		return "low"
	}
}

// ParseConfidence parses "low", "medium" or "high".
func ParseConfidence(s string) (Confidence, error) {
	switch strings.ToLower(s) {
	case "low":
		return Low, nil
	case "medium":
		return Medium, nil
	case "high":
		return High, nil
	}
	return Low, fmt.Errorf("invalid confidence %q (want low, medium or high)", s)
}

// Prompt is a prompt found in source code or in a prompt file.
type Prompt struct {
	File       string     `json:"file"`
	Line       int        `json:"line"` // 1-based
	EndLine    int        `json:"end_line"`
	Context    string     `json:"context"` // e.g. "call client.messages.create"
	Confidence Confidence `json:"-"`
	Text       string     `json:"-"`               // decoded literal, interpolations replaced by {name} slots
	Slots      []string   `json:"slots,omitempty"` // raw interpolated expressions

	// Prompt files (skills, CLAUDE.md, templates) carry a kind, the name and
	// description from their frontmatter, and lint hints. Kind is empty for
	// prompts embedded in code.
	Kind        string            `json:"kind,omitempty"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Hints       []promptfile.Hint `json:"hints,omitempty"`
}

// Options controls a scan.
type Options struct {
	MaxFileSize   int64 // bytes, 0 means default (1 MiB)
	MinConfidence Confidence
}

const defaultMaxFileSize = 1 << 20

// skipDirs are directory names never worth scanning: dependencies, build
// output, and translation/content directories whose long marketing prose
// would otherwise flood the natural-language heuristic.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "__pycache__": true, ".venv": true, "venv": true,
	".idea": true, ".vscode": true,
	"lang": true, "locale": true, "locales": true, "translations": true,
	"i18n": true, "l10n": true,
}

// skipFilePatterns mark files never worth scanning: Storybook stories are
// UI documentation, the rest is generated or minified code.
var skipFilePatterns = []string{
	".stories.", "_pb2.py", "_pb2_grpc.py", ".pb.go", ".min.js", ".generated.",
}

func skipFile(name string) bool {
	l := strings.ToLower(name)
	for _, p := range skipFilePatterns {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

// Scan walks the given paths and extracts prompts from every supported
// source file. Files are processed concurrently; results are returned in
// deterministic (file, line) order.
func Scan(paths []string, opts Options) ([]Prompt, error) {
	maxSize := opts.MaxFileSize
	if maxSize == 0 {
		maxSize = defaultMaxFileSize
	}

	var files []string
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if skipFile(filepath.Base(root)) {
				continue
			}
			if _, ok := promptfile.Detect(root); ok || languageFor(root) != nil {
				files = append(files, root)
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if skipFile(d.Name()) {
				return nil
			}
			if languageFor(path) == nil {
				if _, ok := promptfile.Detect(path); !ok {
					return nil
				}
			}
			if info, err := d.Info(); err == nil && info.Size() > maxSize {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	jobs := make(chan string)
	var mu sync.Mutex
	var prompts []Prompt
	var refs []reference
	var firstErr error

	var wg sync.WaitGroup
	for range max(1, runtime.NumCPU()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newWorker()
			defer w.close()
			for path := range jobs {
				var found []Prompt
				var err error
				if languageFor(path) != nil {
					found, err = w.extractFile(path)
				} else if m, ok := promptfile.Detect(path); ok {
					var p Prompt
					var keep bool
					p, keep, err = LoadPromptFile(path, m)
					if keep {
						found = []Prompt{p}
					}
				}
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				prompts = append(prompts, found...)
				refs = append(refs, w.refs...)
				w.refs = nil
				mu.Unlock()
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	prompts = resolveReferences(prompts, refs, paths)
	addDuplicateHints(prompts)

	filtered := prompts[:0]
	for _, p := range prompts {
		if p.Confidence >= opts.MinConfidence {
			filtered = append(filtered, p)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].File != filtered[j].File {
			return filtered[i].File < filtered[j].File
		}
		return filtered[i].Line < filtered[j].Line
	})
	return filtered, nil
}

// LoadPromptFile reads a prompt file (a skill, a CLAUDE.md, a template) and
// turns it into a Prompt. The whole body is the prompt; frontmatter is
// parsed for its name and description and stripped from the text. keep is
// false when the content does not qualify (no frontmatter where one is
// required, no prose).
func LoadPromptFile(path string, m promptfile.Match) (p Prompt, keep bool, err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Prompt{}, false, err
	}
	f := promptfile.Parse(path, content, m)
	if !promptfile.Accept(f, m) {
		return Prompt{}, false, nil
	}
	// Templates found by location or name are still subject to the content
	// vetoes: a .twig under templates/prompts/ may be an HTML view.
	if f.Kind == promptfile.KindTemplate && f.Confidence < promptfile.High && looksLikeMarkup(f.Body) {
		return Prompt{}, false, nil
	}
	return PromptFromFile(f), true, nil
}

// PromptFromFile wraps a parsed prompt file as a Prompt, linting it against
// the files that sit next to it on disk.
func PromptFromFile(f *promptfile.File) Prompt {
	dir := filepath.Dir(f.Path)
	var stat promptfile.Stat = func(rel string) (bool, bool) {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return false, false
		}
		return true, info.IsDir()
	}
	if f.Path == "" || f.Path == "stdin" {
		stat = nil
	}
	return Prompt{
		File:        f.Path,
		Line:        f.BodyLine,
		EndLine:     f.Lines,
		Context:     f.Context,
		Confidence:  Confidence(f.Confidence),
		Text:        f.Body,
		Kind:        string(f.Kind),
		Name:        f.Name,
		Description: f.Description,
		Hints:       promptfile.Hints(f, stat),
	}
}

// reference is a string literal in source code that names a template file:
// open("prompts/system.md"), Path("summary.jinja").read_text(), ...
type reference struct {
	from     string // source file
	line     int
	target   string // the literal path
	evidence bool   // bound to a prompt-like name or handed to an SDK call
}

// resolveReferences resolves template references against the referencing
// file's directory, then each scan root, then the working directory. A
// resolved file that was already found by location keeps its kind but is
// upgraded with the reference as context; a new file is loaded as a template.
func resolveReferences(prompts []Prompt, refs []reference, roots []string) []Prompt {
	if len(refs) == 0 {
		return prompts
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].from != refs[j].from {
			return refs[i].from < refs[j].from
		}
		return refs[i].line < refs[j].line
	})
	byPath := map[string]int{}
	for i, p := range prompts {
		if p.Kind != "" {
			byPath[cleanPath(p.File)] = i
		}
	}
	for _, r := range refs {
		target, ok := resolveTarget(r, roots)
		if !ok {
			continue
		}
		context := fmt.Sprintf("referenced from %s:%d", r.from, r.line)
		if i, found := byPath[cleanPath(target)]; found {
			if r.evidence && prompts[i].Confidence < High {
				prompts[i].Confidence = High
				prompts[i].Context = context
			}
			continue
		}
		_, strong := promptfile.IsTemplatePath(r.target)
		if !strong && !r.evidence {
			continue
		}
		m := promptfile.Match{Kind: promptfile.KindTemplate, Context: context, Confidence: promptfile.Medium}
		if r.evidence {
			m.Confidence = promptfile.High
		}
		p, keep, err := LoadPromptFile(target, m)
		if err != nil || !keep {
			continue
		}
		byPath[cleanPath(target)] = len(prompts)
		prompts = append(prompts, p)
	}
	return prompts
}

func resolveTarget(r reference, roots []string) (string, bool) {
	rel := filepath.FromSlash(r.target)
	candidates := []string{filepath.Join(filepath.Dir(r.from), rel)}
	for _, root := range roots {
		if info, err := os.Stat(root); err == nil && !info.IsDir() {
			root = filepath.Dir(root)
		}
		candidates = append(candidates, filepath.Join(root, rel))
	}
	candidates = append(candidates, rel)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

func cleanPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// minDuplicateWords is the minimum length of a line for a repeat to mean
// something: short bullets ("run the tests") recur legitimately.
const minDuplicateWords = 6

// addDuplicateHints flags instruction lines repeated verbatim across rules
// files (or twice in the same one). Nested CLAUDE.md files drift apart by
// copy-paste: the same rule in three places is three places to update.
func addDuplicateHints(prompts []Prompt) {
	type site struct {
		idx  int
		line int
	}
	seen := map[string][]site{}
	var order []string
	for i, p := range prompts {
		if p.Kind != string(promptfile.KindRules) {
			continue
		}
		inFence := false
		for n, l := range strings.Split(p.Text, "\n") {
			t := strings.TrimSpace(l)
			if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				inFence = !inFence
				continue
			}
			if inFence || strings.HasPrefix(t, "#") {
				continue
			}
			key := strings.ToLower(strings.Join(strings.Fields(strings.TrimLeft(t, "-*+> \t0123456789.")), " "))
			if len(strings.Fields(key)) < minDuplicateWords {
				continue
			}
			if _, ok := seen[key]; !ok {
				order = append(order, key)
			}
			seen[key] = append(seen[key], site{i, p.Line + n})
		}
	}
	count := map[int]int{}
	for _, key := range order {
		sites := seen[key]
		if len(sites) < 2 {
			continue
		}
		for _, s := range sites {
			if count[s.idx] >= 5 {
				continue
			}
			others := make([]string, 0, len(sites)-1)
			for _, o := range sites {
				if o != s {
					others = append(others, fmt.Sprintf("%s:%d", prompts[o.idx].File, o.line))
				}
			}
			count[s.idx]++
			prompts[s.idx].Hints = append(prompts[s.idx].Hints, promptfile.Hint{
				Rule:     "duplicate-rule",
				Severity: "info",
				Message:  "same rule as " + strings.Join(others, ", ") + " (one place to update is enough)",
				Line:     s.line,
			})
		}
	}
}
