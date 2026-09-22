package bridgefacilitator

import (
	"strings"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"gopkg.in/yaml.v3"
)

type parallelSender struct{ txmanager.Sender }

func (parallelSender) Capacity() int { return 2 }

func TestFactoryRejectsDelegatedOfferSigner(t *testing.T) {
	_, err := factory(yaml.Node{}, solver.Deps{TxManager: parallelSender{}})
	if err == nil || !strings.Contains(err.Error(), "ERC-1271") {
		t.Fatalf("expected actionable delegated offer-signer error, got %v", err)
	}
}
