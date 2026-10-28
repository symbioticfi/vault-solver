package merklehost

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

type indexedChain interface {
	recipes.Reader
	protocolio.BatchReader
	provider.CodeOverrideReader
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
}

// IndexedReader adds indexed publications while preserving the shared RPC reader.
type IndexedReader struct {
	indexedChain
	recipes.PublicationSource
}

func NewIndexedReader(chain indexedChain, source recipes.PublicationSource) *IndexedReader {
	return &IndexedReader{indexedChain: chain, PublicationSource: source}
}
