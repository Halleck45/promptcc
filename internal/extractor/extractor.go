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

// Prompt is a prompt found in source code.
type Prompt struct {
	File       string     `json:"file"`
	Line       int        `json:"line"` // 1-based
	EndLine    int        `json:"end_line"`
	Context    string     `json:"context"` // e.g. "call client.messages.create"
	Confidence Confidence `json:"-"`
	Text       string     `json:"-"`               // decoded literal, interpolations replaced by {name} slots
	Slots      []string   `json:"slots,omitempty"` // raw interpolated expressions
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

// skipFile reports whether a file name is never worth scanning: Storybook
// stories are UI documentation and display fixtures, not model input.
func skipFile(name string) bool {
	return strings.Contains(strings.ToLower(name), ".stories.")
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
			if languageFor(root) != nil && !skipFile(filepath.Base(root)) {
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
			if languageFor(path) == nil || skipFile(d.Name()) {
				return nil
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
	var firstErr error

	var wg sync.WaitGroup
	for range max(1, runtime.NumCPU()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newWorker()
			defer w.close()
			for path := range jobs {
				found, err := w.extractFile(path)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				prompts = append(prompts, found...)
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
