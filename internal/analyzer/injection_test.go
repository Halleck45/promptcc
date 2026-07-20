package analyzer

import "testing"

func TestCountInjections(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantChannels int
		wantSlots    int
	}{
		{"empty", "", 0, 0},
		{"fstring slot", "Hello {user_name}, welcome", 1, 1},
		{"handlebars not double counted as brace", "Hello {{user_name}}", 1, 1},
		{"shell braces not double counted", "Hello ${USER} and $HOME", 1, 2},
		{"dollar amount is not a slot", "refund up to $50 or $5", 0, 0},
		{"printf style", "value is %(count)s items", 1, 1},
		{"double bracket", "insert [[topic]] here", 1, 1},
		{"json example is not a slot", `respond with {"action": "refund", "items": []}`, 0, 0},
		{"placeholder tag counted", "Answer about <topic> briefly", 1, 1},
		{"paired xml tags are structure, not slots", "<context>some text</context>", 0, 0},
		{"mixed xml and placeholder", "<rules>never lie</rules> about <topic>", 1, 1},
		{"two channels", "Hello {name}, insert [[topic]]", 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels, slots, _ := countInjections(tt.in)
			if channels != tt.wantChannels || slots != tt.wantSlots {
				t.Errorf("countInjections(%q) = (%d channels, %d slots), want (%d, %d)",
					tt.in, channels, slots, tt.wantChannels, tt.wantSlots)
			}
		})
	}
}

func TestMaxBraceDepth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"flat text", 0},
		{`{"a": 1}`, 1},
		{`{"a": {"b": [1, 2]}}`, 3},
		{"unbalanced }}} {", 1},
	}
	for _, tt := range tests {
		if got := maxBraceDepth(tt.in); got != tt.want {
			t.Errorf("maxBraceDepth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
