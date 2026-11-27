package connector

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/accessmanager"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipecapability"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"

	binding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// reverts decodes the contracts every connector lane calls; Workflow.Reverts extends it with the
// lane's settlement contracts.
var reverts = RevertDecoder()

// RevertDecoder names reverts from the LiquidLane core, the connector, the Merkle recipe contracts
// (executor, account, AccessManager, capability) and the calling lane's settlement contracts.
func RevertDecoder(integration ...*bind.MetaData) *chain.RevertDecoder {
	return liquidlane.RevertDecoder(append([]*bind.MetaData{
		&binding.LiquidLaneConnectorMetaData, &recipeexecutor.RecipeExecutorMetaData,
		&merkleaccount.MerkleAccountMetaData, &accessmanager.AccessManagerMetaData,
		&recipecapability.RecipeCapabilityMetaData,
	}, integration...)...)
}

// Decoder is Reverts, or the connector and recipe contracts when the lane set none.
func (w *Workflow) Decoder() *chain.RevertDecoder {
	if w.Reverts != nil {
		return w.Reverts
	}
	return reverts
}

// Explain decodes a revert in err with the lane's decoder.
func (w *Workflow) Explain(err error) error { return w.Decoder().Explain(err) }
