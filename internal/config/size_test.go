package config

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		// Suffixed values.
		{"100MB", 100 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"500KB", 500 * 1024, false},
		{"10B", 10, false},

		// Case insensitive.
		{"100mb", 100 * 1024 * 1024, false},
		{"1gb", 1024 * 1024 * 1024, false},
		{"1Tb", 1024 * 1024 * 1024 * 1024, false},

		// Plain integers treated as bytes.
		{"1024", 1024, false},
		{"0", 0, false},

		// Whitespace handling.
		{" 100MB ", 100 * 1024 * 1024, false},
		{"100 MB", 100 * 1024 * 1024, false},

		// Invalid strings.
		{"", 0, true},
		{"abc", 0, true},
		{"MB", 0, true},
		{"-1GB", 0, true},
		{"-100", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseByteSize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseByteSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		got := FormatByteSize(tt.input)
		if got != tt.want {
			t.Errorf("FormatByteSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
