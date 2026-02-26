package db

import "testing"

func TestGlobToLike(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// No wildcards → substring fallback.
		{"hello", "%hello%"},
		// Star maps to %.
		{"*.md", "%.md"},
		{"docs/*.txt", "docs/%.txt"},
		{"*", "%"},
		// Question mark maps to _.
		{"report?.pdf", "report_.pdf"},
		// Mixed.
		{"src/*/?pp", "src/%/_pp"},
		// SQL metacharacters are escaped.
		{"100%", "%100\\%%"},
		{"file_name", "%file\\_name%"},
		// SQL metacharacters + glob wildcards.
		{"100%*.log", "100\\%%.log"},
	}
	for _, tt := range tests {
		got := globToLike(tt.input)
		if got != tt.want {
			t.Errorf("globToLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
