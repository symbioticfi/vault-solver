package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// Dependencies are required only by policies that own position discovery and callback signing.
// Quote/fill policies in the other integrations consume snapshots and need no runtime services.
type Dependencies struct {
	Chain               *chain.Client
	Signer              signer.Signer
	Log                 logr.Logger
	ChainID             int64
	Adapter             common.Address
	Callback            common.Address
	LoadAdapterSnapshot func() (AdapterSnapshot, bool)
	GasAccounting       bool
}
