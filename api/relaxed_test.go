package api

import (
	"encoding/json"
	"testing"

	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/api/rfqbackendinternal"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/api/uniswapxservice"
)

// Every generated client must decode a response that drops a required field or adds an unknown
// one (the tolerance hack/openapi-relax-client.py and OPENAPI_TOLERANT_PROPS provide). This checks
// the behaviour rather than the generator's template text, so a generator bump that leaves the
// relaxer matching nothing fails here instead of shipping a strict client.
func TestGeneratedClientsTolerateDrift(t *testing.T) {
	targets := map[string]any{
		"threef":             &threef.AuctionDto{},
		"rfqbackend":         &rfqbackend.ApprovalCheckResponse{},
		"rfqbackendinternal": &rfqbackendinternal.DiscountsResponse{},
		"lifiorder":          &lifiorder.CompactOrderResponseDto{},
		"uniswapxservice":    &uniswapxservice.DutchOrderEntity{},
	}
	for name, target := range targets {
		for _, body := range []string{`{}`, `{"fieldAddedUpstream":1}`} {
			if err := json.Unmarshal([]byte(body), target); err != nil {
				t.Errorf("%s: decoding %s: %v", name, body, err)
			}
		}
	}
}
