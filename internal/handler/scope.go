package handler

import (
	"regexp"
	"strings"
)

var scopePattern = regexp.MustCompile(`^([a-zA-Z0-9_-]+|\*):r(w)?$`)

// ParseScopes validates a space-separated scope string.
// Valid format: "module:r" or "module:rw" where module is [a-zA-Z0-9_-]+ or "*".
// Returns the individual scope strings and whether they are all valid.
func ParseScopes(scopeStr string) ([]string, bool) {
	scopes := strings.Fields(scopeStr)
	if len(scopes) == 0 {
		return nil, false
	}
	for _, s := range scopes {
		if !scopePattern.MatchString(s) {
			return nil, false
		}
	}
	return scopes, true
}

// CheckScope returns true if the scopes list grants access for the given
// storage path and write flag. It extracts the module (first path segment)
// and matches against scope module + access level.
func CheckScope(scopes []string, storagePath string, write bool) bool {
	// Extract module from path: first segment after leading "/".
	// e.g. "/contacts/foo" → "contacts", "/" → ""
	trimmed := strings.TrimPrefix(storagePath, "/")
	module := trimmed
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		module = trimmed[:idx]
	}

	for _, s := range scopes {
		m := scopePattern.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		scopeModule := m[1]
		hasWrite := m[2] == "w"

		// Module match: exact or wildcard.
		if scopeModule != "*" && scopeModule != module {
			continue
		}

		// Access level: rw grants both, r grants read only.
		if write && !hasWrite {
			continue
		}

		return true
	}
	return false
}
