package connector_test

import (
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/reactor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
)

func TestWorkflowExplainDecodesRevertsThroughSDKWrapping(t *testing.T) {
	t.Parallel()
	transport := errors.New("connection refused")
	lane := &connector.Workflow{Reverts: connector.RevertDecoder(&reactor.ReactorMetaData)}
	for _, tc := range []struct {
		name     string
		workflow *connector.Workflow
		err      error
		want     string
	}{
		{
			// The SDK manager's gas estimate (Sentry VAULT-SOLVER-2Q) keeps the RPC error as its cause.
			name: "SDK gas estimate", workflow: &connector.Workflow{},
			err:  protocolio.Failure("estimate transaction gas", connectortest.Revert("InvalidDiscount()")),
			want: "estimate transaction gas failed: JSON-RPC error rpc_code=3 classification=execution reverted (revert InvalidDiscount())",
		},
		{
			name: "SDK eth_call", workflow: &connector.Workflow{},
			err:  protocolio.Failure("call contract on read RPC", protocolio.CallFailure("eth_call", ethereum.CallMsg{}, nil, connectortest.Revert("UnconfiguredAsset()"))),
			want: "(revert UnconfiguredAsset())",
		},
		{name: "Merkle recipe executor", workflow: &connector.Workflow{}, err: connectortest.Revert("CheckFailed()"), want: "(revert CheckFailed())"},
		{name: "lane settlement contract", workflow: lane, err: connectortest.Revert("InvalidProtocolSignature()"), want: "(revert InvalidProtocolSignature())"},
		{name: "no payload", workflow: &connector.Workflow{}, err: protocolio.Failure("estimate transaction gas", &connectortest.RevertError{}), want: "(node returned no revert data)"},
		{name: "not a revert", workflow: &connector.Workflow{}, err: transport, want: "connection refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.workflow.Explain(tc.err)
			if !strings.Contains(got.Error(), tc.want) || !errors.Is(got, tc.err) {
				t.Fatalf("Explain = %q, want %q wrapping the cause", got, tc.want)
			}
		})
	}
	if err := (&connector.Workflow{}).Explain(transport); err != transport { //nolint:errorlint // identity: a non-revert passes unchanged
		t.Fatal("non-revert was rewrapped")
	}
	if (&connector.Workflow{}).Explain(nil) != nil {
		t.Fatal("nil error became non-nil")
	}
}
