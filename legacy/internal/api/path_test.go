package api

import (
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{"root folder", "/", false},
		{"simple document", "/file.txt", false},
		{"nested document", "/foo/bar/baz.txt", false},
		{"folder path", "/foo/bar/", false},
		{"unicode filename", "/données/日本語.txt", false},
		{"dotfile", "/.hidden", false},
		{"dotfile in folder", "/foo/.gitignore", false},
		{"double dot in filename", "/my..file.txt", false},
		{"triple dot in filename", "/what...no", false},
		{"dots in folder name", "/my..dir/file.txt", false},
		{"backslash in name", `/foo\bar.txt`, false},
		{"max length 512", "/" + strings.Repeat("a", 511), false},

		// Invalid paths
		{"empty path", "", true},
		{"no leading slash", "foo/bar", true},
		{"null byte", "/foo/\x00bar", true},
		{"empty segment double slash", "/foo//bar", true},
		{"dot segment", "/foo/./bar", true},
		{"dotdot segment", "/foo/../bar", true},
		{"dot segment at start", "/./foo", true},
		{"dotdot segment at start", "/../foo", true},
		{"dot segment at end", "/foo/.", true},
		{"dotdot as only segment", "/..", true},
		{"dot as only segment", "/.", true},
		{"over 512 chars", "/" + strings.Repeat("a", 512), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
