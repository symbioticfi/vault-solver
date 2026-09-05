package lifi

import (
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
)

// Non-EVM submissions arrive on the feed constantly and are never fillable here, so they are
// expected traffic: logged at debug with the reason, never at error (which would reach Sentry).
func TestParseOrderMessageIgnoresUnsupportedOrdersAtDebug(t *testing.T) {
	cfg := testLifiConfig()
	raw := mutatedTestOrderJSON(t, cfg, func(body map[string]any) {
		body["orderType"] = "GaslessCrosschainOrder"
	})

	for _, tc := range []struct {
		name      string
		verbosity int
		wantLine  bool
	}{
		{name: "production verbosity drops it", verbosity: 0, wantLine: false},
		{name: "debug verbosity shows the reason", verbosity: 1, wantLine: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			solver := &Solver{
				cfg:     cfg,
				chainID: 11155111,
				log: funcr.NewJSON(func(entry string) { logs = append(logs, entry) },
					funcr.Options{Verbosity: tc.verbosity}),
			}
			if order := solver.parseOrderMessage(orderMessage{Event: orderSubmitEvent, Data: raw}); order != nil {
				t.Fatalf("parseOrderMessage() = %+v, want ignored order", order)
			}
			logged := strings.Join(logs, "\n")
			if strings.Contains(logged, `"error"`) {
				t.Fatalf("unsupported order logged at error: %s", logged)
			}
			gotLine := strings.Contains(logged, "order feed: ignored unsupported order") &&
				strings.Contains(logged, "unsupported non-onchain order type")
			if gotLine != tc.wantLine {
				t.Fatalf("debug line present = %v, want %v; logs = %q", gotLine, tc.wantLine, logged)
			}
		})
	}
}
