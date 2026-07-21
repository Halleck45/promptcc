package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scanTestdata(t *testing.T, minConf Confidence) []Prompt {
	t.Helper()
	prompts, err := Scan([]string{"testdata"}, Options{MinConfidence: minConf})
	if err != nil {
		t.Fatal(err)
	}
	return prompts
}

func promptsForFile(prompts []Prompt, name string) []Prompt {
	var out []Prompt
	for _, p := range prompts {
		if filepath.Base(p.File) == name {
			out = append(out, p)
		}
	}
	return out
}

func TestScanPython(t *testing.T) {
	found := promptsForFile(scanTestdata(t, Low), "sample.py")
	if len(found) != 3 {
		for _, p := range found {
			t.Logf("found: %s:%d [%s] %s", p.File, p.Line, p.Confidence, p.Context)
		}
		t.Fatalf("found %d prompts, want 3 (system, user content, instructions var)", len(found))
	}

	system := found[0]
	if system.Confidence != Medium && system.Confidence != High {
		t.Errorf("SYSTEM_PROMPT confidence = %s, want medium or high", system.Confidence)
	}
	if !strings.Contains(system.Text, "If the user asks for a refund") {
		t.Errorf("system prompt text not decoded:\n%s", system.Text)
	}
	if !strings.Contains(system.Text, "{company}") {
		t.Errorf("f-string interpolation should become a {company} slot:\n%s", system.Text)
	}
	if len(system.Slots) != 1 || system.Slots[0] != "{company}" {
		t.Errorf("Slots = %v, want the raw interpolation", system.Slots)
	}

	var userContent *Prompt
	for i := range found {
		if found[i].Confidence == High && strings.Contains(found[i].Text, "Answer this question") {
			userContent = &found[i]
		}
	}
	if userContent == nil {
		t.Fatal("f-string inside messages.create not found with high confidence")
	}
	if !strings.Contains(userContent.Context, "messages.create") {
		t.Errorf("Context = %q, want the SDK callee", userContent.Context)
	}
	if !strings.Contains(userContent.Text, "{question}") {
		t.Errorf("interpolation not slotted:\n%s", userContent.Text)
	}
}

func TestScanPythonShortStringsIgnored(t *testing.T) {
	for _, p := range promptsForFile(scanTestdata(t, Low), "sample.py") {
		if strings.Contains(p.Text, "hi") && len(p.Text) < 12 {
			t.Errorf("short string %q should be ignored", p.Text)
		}
		if strings.Contains(p.Text, "claude-sonnet") {
			t.Errorf("model id %q should not be a prompt", p.Text)
		}
	}
}

func TestScanTypeScript(t *testing.T) {
	found := promptsForFile(scanTestdata(t, Low), "sample.ts")
	if len(found) != 2 {
		for _, p := range found {
			t.Logf("found: %s:%d [%s] %s %q", p.File, p.Line, p.Confidence, p.Context, p.Text)
		}
		t.Fatalf("found %d prompts, want 2 (system template, user content)", len(found))
	}

	system := found[0]
	if !strings.Contains(system.Text, "{teamName}") {
		t.Errorf("template substitution should become a slot:\n%s", system.Text)
	}
	if system.Confidence < Medium {
		t.Errorf("systemPrompt confidence = %s, want at least medium", system.Confidence)
	}

	user := found[1]
	if user.Confidence != High {
		t.Errorf("user content confidence = %s, want high (inside messages.create)", user.Confidence)
	}
	if !strings.Contains(user.Text, "{ticket}") {
		t.Errorf("interpolation not slotted:\n%s", user.Text)
	}
}

func TestScanPHP(t *testing.T) {
	found := promptsForFile(scanTestdata(t, Low), "sample.php")
	if len(found) != 2 {
		for _, p := range found {
			t.Logf("found: %s:%d [%s] %s %q", p.File, p.Line, p.Confidence, p.Context, p.Text)
		}
		t.Fatalf("found %d prompts, want 2 (heredoc, user content)", len(found))
	}

	heredoc := found[0]
	if !strings.Contains(heredoc.Text, "If the invoice is overdue") {
		t.Errorf("heredoc not decoded:\n%s", heredoc.Text)
	}
	if !strings.Contains(heredoc.Text, "{companyName}") {
		t.Errorf("PHP interpolation should become a slot:\n%s", heredoc.Text)
	}
	if heredoc.Confidence < Medium {
		t.Errorf("heredoc confidence = %s, want at least medium (assigned to $systemPrompt)", heredoc.Confidence)
	}

	user := found[1]
	if user.Confidence != High {
		t.Errorf("user content confidence = %s, want high (inside chat()->create)", user.Confidence)
	}
	if !strings.Contains(user.Text, "{customerName}") {
		t.Errorf("encapsed interpolation not slotted:\n%s", user.Text)
	}
}

func TestScanMinConfidenceFilters(t *testing.T) {
	all := scanTestdata(t, Low)
	high := scanTestdata(t, High)
	if len(high) >= len(all) {
		t.Errorf("min-confidence high should filter: %d >= %d", len(high), len(all))
	}
	for _, p := range high {
		if p.Confidence != High {
			t.Errorf("%s:%d has confidence %s, want high", p.File, p.Line, p.Confidence)
		}
	}
}

func TestScanDeterministicOrder(t *testing.T) {
	a := scanTestdata(t, Low)
	b := scanTestdata(t, Low)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic count: %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i].File != b[i].File || a[i].Line != b[i].Line {
			t.Fatalf("non-deterministic order at %d: %s:%d != %s:%d",
				i, a[i].File, a[i].Line, b[i].File, b[i].Line)
		}
	}
}

func TestScanSkipsIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `const systemPrompt = "If asked about billing, escalate to a human. Never guess numbers.";`
	if err := os.WriteFile(filepath.Join(nested, "index.ts"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.ts"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	prompts, err := Scan([]string{dir}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 1 {
		t.Fatalf("found %d prompts, want 1 (node_modules must be skipped)", len(prompts))
	}
}

func TestScanSingleFileArgument(t *testing.T) {
	prompts, err := Scan([]string{filepath.Join("testdata", "sample.py")}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) == 0 {
		t.Fatal("scanning a single file should work")
	}
}

func TestSlotName(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"{company}", "company"},
		{"${user.name}", "user_name"},
		{"$customerName", "customerName"},
		{"{a + b}", "a_b"},
		{"{}", "value_1"},
	}
	for _, tt := range tests {
		if got := slotName(tt.expr, 1); got != tt.want {
			t.Errorf("slotName(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestScanLaravelValidationIsNotAPrompt(t *testing.T) {
	// Regression: Laravel FormRequest rules and messages use "prompt" as a
	// key ('prompt' => 'required|string', 'prompt.required_without' => ...)
	// but none of these strings are prompts.
	found := promptsForFile(scanTestdata(t, Low), "laravel_request.php")
	for _, p := range found {
		t.Errorf("false positive: %s:%d [%s] %s %q", p.File, p.Line, p.Confidence, p.Context, p.Text)
	}
}

func TestLooksLikeInstruction(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"required_without:prompt_template|string|max:4000", false},
		{"gpt-4o", false},
		{"Answer the question briefly and cite sources.", true},
		{"You are a helpful billing assistant.", true},
		{"a b c", false},
	}
	for _, tt := range tests {
		if got := looksLikeInstruction(tt.in); got != tt.want {
			t.Errorf("looksLikeInstruction(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestScanSQLIsNotAPrompt(t *testing.T) {
	// Regression: SQL CASE WHEN ... THEN ... ELSE reads like prose and is
	// full of decision keywords, but it is not a prompt.
	found := promptsForFile(scanTestdata(t, Low), "sql_controller.php")
	for _, p := range found {
		t.Errorf("false positive: %s:%d [%s] %s %q", p.File, p.Line, p.Confidence, p.Context, p.Text)
	}
}

func TestLooksLikeSQL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"aggregation query", "SELECT SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS completed, COUNT(*) AS total FROM items", true},
		{"case when like", "CASE WHEN payload LIKE '%AudioStep%' THEN 'audio' ELSE 'other' END AS step", true},
		{"prose with select", "Select the best answer from the list and explain why.", false},
		{"prompt with conditions", "If the user asks for a refund, then check the date. When angry, escalate.", false},
		{"plain prompt", "You are a support agent. Never guess.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeSQL(tt.in); got != tt.want {
				t.Errorf("looksLikeSQL(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestScanDocumentationIsNotAPrompt(t *testing.T) {
	// Regression: artisan signatures, command descriptions, step
	// self-documentation (summary:, notes:) and Python docstrings are
	// documentation, not prompts.
	all := scanTestdata(t, Low)
	for _, name := range []string{"artisan_command.php", "step_description.php", "docstrings.py"} {
		for _, p := range promptsForFile(all, name) {
			t.Errorf("false positive: %s:%d [%s] %s %q", p.File, p.Line, p.Confidence, p.Context, p.Text)
		}
	}
}

func TestLooksLikeSpec(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"artisan options", "app:generate\n  {--service=openai : which provider}\n  {--dry-run : do nothing}", true},
		{"artisan single line", "app:test {--template : use a template}", true},
		{"prompt with json example", "Respond with JSON:\n{\"action\": \"refund\"}\n{\"action\": \"escalate\"}", false},
		{"plain prompt", "You are a support agent. If asked, escalate.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeSpec(tt.in); got != tt.want {
				t.Errorf("looksLikeSpec(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLastIdentSegment(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"'prompt'", "prompt"},
		{"$systemPrompt", "systemprompt"},
		{"$this->systemPrompt", "systemprompt"},
		{"prompt.required_without", "required_without"},
		{"self::PROMPT", "prompt"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := lastIdentSegment(tt.in); got != tt.want {
			t.Errorf("lastIdentSegment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
