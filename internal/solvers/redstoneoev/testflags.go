package redstoneoev

// testflags.go reads solver-owned dev/test knobs from env vars at point of use. Production leaves them unset.
// Malformed values fail closed (error) so a typo can't silently widen scope.

import (
	"os"
	"strings"

	"github.com/go-errors/errors"
)

const (
	envDryRun = "OEV_DRY_RUN" // "true"/"1" → observe mode: sign + log would-bids, never send
)

// dryRunEnv reports whether OEV_DRY_RUN puts the bot in observe mode — sign + log each would-bid but never
// send it ("true"/"1", case-insensitive); unset/false → false; a malformed value → error.
func dryRunEnv() (bool, error) { return envBool(envDryRun) }

// envBool reads a boolean env flag, failing closed: unset/""/"false"/"0" → false; "true"/"1" → true
// (case-insensitive, trimmed); any other SET value → error — so a typo (e.g. OEV_DRY_RUN=ture) can't
// silently flip the bot into live bidding instead of the intended observe mode.
func envBool(key string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "", "false", "0":
		return false, nil
	case "true", "1":
		return true, nil
	default:
		return false, errors.Errorf("%s: invalid bool %q (want true/1 or false/0)", key, os.Getenv(key))
	}
}
