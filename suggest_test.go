package main

import "testing"

func TestSuggestCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Close typo (1 edit) → suggestion
		{"depoy", "deploy"},   // delete one char: dep-o-y → deploy
		{"stars", "start"},    // substitute last char: star-s → start
		{"lisst", "list"},     // delete extra char: li-ss-t → list
		{"sttatus", "status"}, // delete extra char: s-tt-atus → status

		// Two-edit typo within threshold for longer inputs (threshold = len/3 ≥ 2)
		{"deplyo", "deploy"}, // swap last two chars: len=6, threshold=2

		// Prefix match wins outright
		{"setup-t", "setup-traefik"},
		{"setup-d", "setup-dns"},
		{"import", "import-traefik"},

		// Unrelated input → no suggestion
		{"xyzzy", ""},
		{"frobnicate", ""},
		{"qqq", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := suggestCommand(tt.input)
			if got != tt.want {
				t.Errorf("suggestCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"kitten", "sitting", 3},
		{"deploy", "deplyo", 2},
		{"status", "sttatus", 1},
		{"start", "stars", 1},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
