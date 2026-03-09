package api

import "testing"

func TestCheckScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		path   string
		write  bool
		want   bool
	}{
		{"exact module read", []string{"contacts:r"}, "/contacts/card.vcf", false, true},
		{"exact module write", []string{"contacts:rw"}, "/contacts/card.vcf", true, true},
		{"read scope denies write", []string{"contacts:r"}, "/contacts/card.vcf", true, false},
		{"wrong module", []string{"contacts:rw"}, "/calendar/event.ics", true, false},
		{"wildcard read", []string{"*:r"}, "/anything/file.txt", false, true},
		{"wildcard write", []string{"*:rw"}, "/anything/file.txt", true, true},
		{"wildcard read denies write", []string{"*:r"}, "/anything/file.txt", true, false},

		// Public paths: photos:rw must grant access to public/photos/.
		{"public path read with module scope", []string{"photos:r"}, "/public/photos/pic.jpg", false, true},
		{"public path write with module scope", []string{"photos:rw"}, "/public/photos/pic.jpg", true, true},
		{"public path read denied wrong module", []string{"contacts:r"}, "/public/photos/pic.jpg", false, false},
		{"public folder listing with module scope", []string{"photos:r"}, "/public/photos/", false, true},
		{"wildcard covers public path", []string{"*:rw"}, "/public/photos/pic.jpg", true, true},

		// Multiple scopes.
		{"multiple scopes first matches", []string{"photos:rw", "contacts:r"}, "/photos/pic.jpg", true, true},
		{"multiple scopes second matches", []string{"photos:rw", "contacts:r"}, "/contacts/card.vcf", false, true},

		// Root path.
		{"root folder", []string{"*:r"}, "/", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckScope(tt.scopes, tt.path, tt.write)
			if got != tt.want {
				t.Errorf("CheckScope(%v, %q, write=%v) = %v, want %v",
					tt.scopes, tt.path, tt.write, got, tt.want)
			}
		})
	}
}

func TestParseScopes(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
		count int
	}{
		{"contacts:r", true, 1},
		{"contacts:rw", true, 1},
		{"*:r", true, 1},
		{"*:rw", true, 1},
		{"photos:r contacts:rw", true, 2},
		{"", false, 0},
		{"invalid", false, 0},
		{"contacts:x", false, 0},
		{"contacts:", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			scopes, ok := ParseScopes(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseScopes(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && len(scopes) != tt.count {
				t.Errorf("ParseScopes(%q) returned %d scopes, want %d", tt.input, len(scopes), tt.count)
			}
		})
	}
}
