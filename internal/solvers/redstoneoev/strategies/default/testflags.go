package defaultstrategy

import (
	"os"
	"strings"

	"github.com/go-errors/errors"
)

const (
	envTestMonitor = "OEV_TEST_MONITOR" // "true"/"1" → use Sepolia harness on-chain Morpho monitor
)

// testMonitorFromEnv reads the default-strategy-only Sepolia harness toggle.
func testMonitorFromEnv() (bool, error) {
	return envBool(envTestMonitor)
}

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
