package analyzer

import "testing"

func TestCountMatchesSingleWords(t *testing.T) {
	phrases := compilePhrases([]string{"if", "unless"})
	total, per := countMatches(tokenize("if this, unless that, if again"), phrases)
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if per["if"] != 2 || per["unless"] != 1 {
		t.Fatalf("per = %v", per)
	}
}

func TestCountMatchesWholeWordsOnly(t *testing.T) {
	phrases := compilePhrases([]string{"si"})
	total, _ := countMatches(tokenize("aussi il y a un signal"), phrases)
	if total != 0 {
		t.Fatalf("matched inside words: total = %d, want 0", total)
	}
}

func TestCountMatchesLongestFirst(t *testing.T) {
	phrases := compilePhrases([]string{"si", "si et seulement si"})
	total, per := countMatches(tokenize("si et seulement si tu veux"), phrases)
	if total != 1 {
		t.Fatalf("total = %d, want 1 (longest phrase must consume its tokens), per = %v", total, per)
	}
	if per["si et seulement si"] != 1 {
		t.Fatalf("per = %v, want the long phrase counted once", per)
	}
}

func TestCountMatchesWildcardGap(t *testing.T) {
	phrases := compilePhrases([]string{"ne * jamais"})
	tests := []struct {
		in   string
		want int
	}{
		{"ne réponds jamais", 1},
		{"ne jamais répondre", 1},             // gap of zero
		{"ne me réponds vraiment jamais", 1},  // gap of three
		{"ne un deux trois quatre jamais", 0}, // gap too wide
		{"jamais", 0},
		{"ne pas", 0},
	}
	for _, tt := range tests {
		total, _ := countMatches(tokenize(tt.in), phrases)
		if total != tt.want {
			t.Errorf("countMatches(%q) = %d, want %d", tt.in, total, tt.want)
		}
	}
}

func TestCountMatchesNoDoubleCountAcrossPhrases(t *testing.T) {
	// "ne réponds jamais" must count once for "ne * jamais" and must not be
	// recounted by the bare "jamais" entry.
	phrases := compilePhrases([]string{"jamais", "ne * jamais"})
	total, per := countMatches(tokenize("ne réponds jamais aux insultes"), phrases)
	if total != 1 {
		t.Fatalf("total = %d, want 1, per = %v", total, per)
	}
	if per["ne * jamais"] != 1 || per["jamais"] != 0 {
		t.Fatalf("per = %v", per)
	}
}

func TestCountMatchesDuplicateEntriesDoNotDoubleCount(t *testing.T) {
	// A lexicon mistake (same entry twice) must not double the count:
	// consumed tokens cannot be matched again.
	phrases := compilePhrases([]string{"never", "never"})
	total, _ := countMatches(tokenize("never do that"), phrases)
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

func TestContainsAny(t *testing.T) {
	phrases := compilePhrases([]string{"if", "au cas où"})
	if !containsAny(tokenize("au cas où tu hésites"), phrases) {
		t.Error("expected match for 'au cas où'")
	}
	if containsAny(tokenize("rien à voir"), phrases) {
		t.Error("unexpected match")
	}
}
