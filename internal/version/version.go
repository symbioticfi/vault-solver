// Package version exposes build metadata, injected at link time via -ldflags.
package version

import "runtime"

// These are overridden at build time, e.g.:
//
//	go build -ldflags "-X github.com/symbioticfi/vault-solver/internal/version.Version=v0.1.0 \
//	  -X github.com/symbioticfi/vault-solver/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/symbioticfi/vault-solver/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	// Version is the semantic version of the build ("dev" for local builds).
	Version = "dev"
	// Commit is the short git SHA of the build.
	Commit = "none"
	// Date is the RFC3339 UTC build timestamp.
	Date = "unknown"
)

// String renders a single-line, human-readable version banner.
func String() string {
	return "vault-solver " + Version + " (commit " + Commit + ", built " + Date + ", " + runtime.Version() + ")"
}
