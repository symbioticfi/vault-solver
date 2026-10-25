// Package merklefunding reads shared account funding for Merkle LL and 3F flows.
package merklefunding

import (
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/merkle/fundinglens"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

// Source is a reviewed scalar capacity query, denominated in the funding asset.
type Source struct {
	ID     string `yaml:"id" json:"id"`
	Target string `yaml:"target" json:"target"`
	Data   string `yaml:"data" json:"data"`
	Offset uint64 `yaml:"offset" json:"offset"`
}

// Entry binds a reviewed recipe's disjoint funding sources to an account/asset pair.
type Entry struct {
	Account string   `yaml:"account" json:"account"`
	Asset   string   `yaml:"asset" json:"asset"`
	Lens    string   `yaml:"lens" json:"lens"`
	Sources []Source `yaml:"sources" json:"sources"`
}

// Config is optional; unconfigured pairs retain cash-only capacity.
type Config []Entry

// Reader is the block-pinned eth_call surface shared by both consumers.
type Reader interface {
	CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error)
}

var fundingBinding = fundinglens.NewMerkleFundingLens()
var tokenBinding = erc4626.NewIERC4626()

// Validate rejects ambiguous routes and invalid/duplicate source configuration.
func (c Config) Validate() error {
	if len(c) > 64 {
		return errors.New("merkleFunding exceeds 64 account/asset pairs")
	}
	pairs := make(map[[2]common.Address]bool)
	for _, entry := range c {
		account, err := parse.NonZeroAddress(entry.Account, "merkleFunding.account")
		if err != nil {
			return err
		}
		asset, err := parse.NonZeroAddress(entry.Asset, "merkleFunding.asset")
		if err != nil {
			return err
		}
		if _, err = parse.NonZeroAddress(entry.Lens, "merkleFunding.lens"); err != nil {
			return err
		}
		key := [2]common.Address{account, asset}
		if pairs[key] {
			return errors.New("duplicate merkleFunding account/asset")
		}
		pairs[key] = true
		if _, err = entry.sources(); err != nil {
			return err
		}
	}
	return nil
}

func (e Entry) sources() ([]fundinglens.IMerkleFundingLensSource, error) {
	if len(e.Sources) > 32 {
		return nil, errors.New("merkleFunding exceeds 32 sources")
	}
	ids := make(map[common.Hash]bool)
	queries := make(map[string]bool)
	out := make([]fundinglens.IMerkleFundingLensSource, 0, len(e.Sources))
	for _, source := range e.Sources {
		rawID, err := hexutil.Decode(source.ID)
		if err != nil || len(rawID) != 32 || common.BytesToHash(rawID) == (common.Hash{}) {
			return nil, errors.New("invalid merkleFunding source id")
		}
		target, err := parse.NonZeroAddress(source.Target, "merkleFunding.source.target")
		if err != nil {
			return nil, err
		}
		data, err := hexutil.Decode(source.Data)
		if err != nil || len(data) < 4 || len(data) > 8192 || source.Offset > 65536 || source.Offset%32 != 0 {
			return nil, errors.New("invalid merkleFunding source query")
		}
		id, query := common.BytesToHash(rawID), target.Hex()+hexutil.Encode(data)
		if ids[id] || queries[query] {
			return nil, errors.New("duplicate merkleFunding source")
		}
		ids[id], queries[query] = true, true
		out = append(out, fundinglens.IMerkleFundingLensSource{Id: id, Target: target, Data: data, Offset: new(big.Int).SetUint64(source.Offset)})
	}
	return out, nil
}

// Available returns cash plus reviewed synchronous capacity. Configured failures never fall back.
func (c Config) Available(ctx context.Context, chain Reader, account, asset common.Address, block *big.Int) (*big.Int, error) {
	if block == nil {
		return nil, errors.New("merkleFunding requires a pinned block")
	}
	target, data := asset, tokenBinding.PackBalanceOf(account)
	for _, entry := range c {
		if common.HexToAddress(entry.Account) != account || common.HexToAddress(entry.Asset) != asset {
			continue
		}
		sources, err := entry.sources()
		if err != nil {
			return nil, err
		}
		target = common.HexToAddress(entry.Lens)
		data, err = fundingBinding.TryPackGetAvailableAssets(account, asset, sources)
		if err != nil {
			return nil, errors.Errorf("encode merkleFunding: %w", err)
		}
		break
	}
	raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &target, Data: data}, block)
	if err != nil {
		return nil, errors.Errorf("read merkleFunding: %w", err)
	}
	if len(raw) != 32 {
		return nil, errors.New("invalid merkleFunding scalar reply")
	}
	return new(big.Int).SetBytes(raw), nil
}
