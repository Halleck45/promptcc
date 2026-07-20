package extractor

import (
	"path/filepath"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsphp "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tstypescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// language describes how to find and decode prompt strings for one grammar.
type language struct {
	name string
	ptr  unsafe.Pointer

	// stringRoots are node kinds that start a string literal. The extractor
	// never descends into a string root, so nested literals (inside
	// interpolations) are not double counted.
	stringRoots map[string]bool

	// literalKinds are nodes inside a string root whose text is literal
	// content, and containerKinds are structural nodes to descend into
	// (e.g. PHP heredoc_body). Any other named node inside a string root is
	// an interpolation slot. delimiterKinds are neither content nor slots.
	literalKinds   map[string]bool
	containerKinds map[string]bool
	delimiterKinds map[string]bool

	// assignmentKinds map a node kind to the field holding the bound name.
	assignmentKinds map[string]string
	// keyedKinds map a node kind to the field holding the key or argument
	// name (keyword arguments, object keys).
	keyedKinds map[string]string
	// callKinds map a call node kind to a function that renders its callee.
	callKinds map[string]func(n *sitter.Node, src []byte) string
}

func fieldText(n *sitter.Node, field string, src []byte) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return c.Utf8Text(src)
}

var pythonLang = &language{
	name: "python",
	ptr:  tspython.Language(),
	stringRoots: map[string]bool{
		"string": true, "concatenated_string": true,
	},
	literalKinds:   map[string]bool{"string_content": true, "escape_sequence": true},
	delimiterKinds: map[string]bool{"string_start": true, "string_end": true},
	assignmentKinds: map[string]string{
		"assignment": "left",
	},
	keyedKinds: map[string]string{
		"keyword_argument": "name",
		"pair":             "key",
	},
	callKinds: map[string]func(n *sitter.Node, src []byte) string{
		"call": func(n *sitter.Node, src []byte) string {
			return fieldText(n, "function", src)
		},
	},
}

var typescriptLang = &language{
	name: "typescript",
	ptr:  tstypescript.LanguageTypescript(),
	stringRoots: map[string]bool{
		"string": true, "template_string": true,
	},
	literalKinds:   map[string]bool{"string_fragment": true, "escape_sequence": true},
	delimiterKinds: map[string]bool{},
	assignmentKinds: map[string]string{
		"variable_declarator":   "name",
		"assignment_expression": "left",
	},
	keyedKinds: map[string]string{
		"pair": "key",
	},
	callKinds: map[string]func(n *sitter.Node, src []byte) string{
		"call_expression": func(n *sitter.Node, src []byte) string {
			return fieldText(n, "function", src)
		},
	},
}

var tsxLang = func() *language {
	l := *typescriptLang
	l.name = "tsx"
	l.ptr = tstypescript.LanguageTSX()
	return &l
}()

var phpLang = &language{
	name: "php",
	ptr:  tsphp.LanguagePHP(),
	stringRoots: map[string]bool{
		"string": true, "encapsed_string": true, "heredoc": true, "nowdoc": true,
	},
	literalKinds: map[string]bool{
		"string_content": true, "escape_sequence": true, "nowdoc_string": true,
	},
	containerKinds: map[string]bool{
		"heredoc_body": true, "nowdoc_body": true,
	},
	delimiterKinds: map[string]bool{
		"heredoc_start": true, "heredoc_end": true,
		"nowdoc_start": true, "nowdoc_end": true,
	},
	assignmentKinds: map[string]string{
		"assignment_expression": "left",
	},
	keyedKinds: map[string]string{
		"named_argument": "name",
	},
	callKinds: map[string]func(n *sitter.Node, src []byte) string{
		"function_call_expression": func(n *sitter.Node, src []byte) string {
			return fieldText(n, "function", src)
		},
		"member_call_expression": func(n *sitter.Node, src []byte) string {
			return fieldText(n, "object", src) + "->" + fieldText(n, "name", src)
		},
		"scoped_call_expression": func(n *sitter.Node, src []byte) string {
			return fieldText(n, "scope", src) + "::" + fieldText(n, "name", src)
		},
		"object_creation_expression": func(n *sitter.Node, src []byte) string {
			return "new " + firstNamedChildText(n, src)
		},
	},
}

func firstNamedChildText(n *sitter.Node, src []byte) string {
	if c := n.NamedChild(0); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

var extensions = map[string]*language{
	".py":  pythonLang,
	".ts":  typescriptLang,
	".mts": typescriptLang,
	".cts": typescriptLang,
	".tsx": tsxLang,
	".php": phpLang,
}

// languageFor returns the language for a file path, or nil if unsupported.
func languageFor(path string) *language {
	return extensions[strings.ToLower(filepath.Ext(path))]
}
