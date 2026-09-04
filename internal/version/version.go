// Package version exposes link-time build metadata.
package version

import "runtime"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func String() string {
	return "vault-solver " + Version + " (commit " + Commit + ", built " + Date + ", " + runtime.Version() + ")"
}
