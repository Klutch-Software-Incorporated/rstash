package config

import "runtime/debug"

// Version is the build version string reported by `rstash --version`, the
// admin status page, and audit/startup logs.
//
// Release and container builds inject it at link time via
//
//	-ldflags "-X rstash/internal/config.Version=<git describe>"
//
// (see Taskfile.yml and Dockerfile), yielding "v0.4.1" on a tagged commit or
// "v0.4.1-7-gabc1234" on an untagged main build — always traceable to a commit.
//
// When it isn't injected — a bare `go build`/`go run` during development, or a
// `go install` of the module — init() derives it from the module/VCS metadata
// Go embeds in every binary, so the server reports a real, traceable value
// instead of a meaningless "dev".
var Version string

func init() {
	if Version != "" {
		return // injected via -ldflags
	}
	Version = versionFromBuildInfo()
}

// versionFromBuildInfo derives a version from the build's embedded module and
// VCS metadata (runtime/debug). It returns "unknown" only when no metadata is
// available at all — e.g. built from a source archive outside a git checkout
// with no ldflags override.
func versionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	// Prefer the embedded VCS metadata when present — it's only stamped for a
	// build from a source checkout, and yields a clean short SHA. We check this
	// *before* Main.Version because Go stamps the main module with an ugly
	// pseudo-version ("v0.0.0-<timestamp>-<sha>") for such builds, which reads
	// like a real v0.0.0 release.
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if dirty {
			rev += "-dirty"
		}
		return rev
	}

	// No VCS metadata: this is a `go install rstash@v0.4.1` build (installed
	// from the module cache, not a checkout), where Main.Version is the clean
	// resolved tag. "(devel)" means even that is absent.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	return "unknown"
}
