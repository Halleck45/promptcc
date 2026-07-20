package analyzer

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"simple", "If the user asks", []string{"if", "the", "user", "asks"}},
		{"accents kept", "dès que possible", []string{"dès", "que", "possible"}},
		{"apostrophe kept inside", "utilise l'outil", []string{"utilise", "l'outil"}},
		{"typographic apostrophe normalized", "l’outil", []string{"l'outil"}},
		{"hyphen kept inside", "comporte-toi bien", []string{"comporte-toi", "bien"}},
		{"punctuation split", "si oui, alors non.", []string{"si", "oui", "alors", "non"}},
		{"case folded", "NEVER Do THAT", []string{"never", "do", "that"}},
		{"leading trailing quotes trimmed", "'quoted' -dash-", []string{"quoted", "dash"}},
		{"digits kept", "30 jours", []string{"30", "jours"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
