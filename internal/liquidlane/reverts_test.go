package liquidlane

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/reactor"
)

func TestRevertDecoderNamesCoreAndIntegrationErrors(t *testing.T) {
	t.Parallel()
	decoder := RevertDecoder(&reactor.ReactorMetaData)
	for signature, want := range map[string]string{
		"InvalidDiscount()":          "InvalidDiscount()",          // LiquidLaneAdapter
		"InvalidProtocolSignature()": "InvalidProtocolSignature()", // Reactor, passed by the integration
	} {
		if got := decoder.Describe(crypto.Keccak256([]byte(signature))[:4]); got != want {
			t.Fatalf("%s: Describe = %q, want %q", signature, got, want)
		}
	}
}
