// Package merklefunding reads shared account funding for Merkle LL and 3F flows.
package merklefunding

import (
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
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
	Sources []Source `yaml:"sources" json:"sources"`
}

// Config is optional; unconfigured pairs retain cash-only capacity.
type Config []Entry

// Reader is the block-pinned eth_call surface shared by both consumers.
type Reader interface {
	CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error)
}

var multicallBinding = multicall3.NewMulticall3()
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

func (e Entry) sources() ([]multicall3.Multicall3Call3, error) {
	if len(e.Sources) > 32 {
		return nil, errors.New("merkleFunding exceeds 32 sources")
	}
	ids := make(map[common.Hash]bool)
	queries := make(map[string]bool)
	out := make([]multicall3.Multicall3Call3, 0, len(e.Sources))
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
		out = append(out, multicall3.Multicall3Call3{Target: target, CallData: data})
	}
	return out, nil
}

// Available returns cash plus reviewed synchronous capacity. Configured failures never fall back.
func (c Config) Available(ctx context.Context, chain Reader, account, asset common.Address, block *big.Int) (*big.Int, error) {
	if block == nil || block.Sign() < 0 {
		return nil, errors.New("merkleFunding requires a pinned block")
	}
	target, data := asset, tokenBinding.PackBalanceOf(account)
	calls := []multicall3.Multicall3Call3{{Target: asset, CallData: data}}
	var sources []Source
	for _, entry := range c {
		if common.HexToAddress(entry.Account) != account || common.HexToAddress(entry.Asset) != asset {
			continue
		}
		queries, err := entry.sources()
		if err != nil {
			return nil, err
		}
		calls = append(calls, queries...)
		sources = entry.Sources
		break
	}
	if len(sources) > 0 {
		target = common.HexToAddress(protocolio.StandardMulticall3)
		data = multicallBinding.PackAggregate3(calls)
	}
	raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &target, Data: data}, block)
	if err != nil {
		return nil, errors.Errorf("read merkleFunding: %w", err)
	}
	if len(sources) == 0 {
		if len(raw) != 32 {
			return nil, errors.New("invalid merkleFunding cash reply")
		}
		return new(big.Int).SetBytes(raw), nil
	}
	results, err := multicallBinding.UnpackAggregate3(raw)
	if err != nil {
		return nil, errors.Errorf("decode merkleFunding: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.New("merkleFunding result count mismatch")
	}
	total := new(big.Int)
	for i, result := range results {
		if !result.Success {
			return nil, errors.Errorf("merkleFunding call %d failed", i)
		}
		offset := uint64(0)
		if i == 0 {
			if len(result.ReturnData) != 32 {
				return nil, errors.New("invalid merkleFunding cash reply")
			}
		} else {
			offset = sources[i-1].Offset
			if uint64(len(result.ReturnData)) < offset+32 {
				return nil, errors.New("invalid merkleFunding source reply")
			}
		}
		total.Add(total, new(big.Int).SetBytes(result.ReturnData[offset:offset+32]))
		if total.BitLen() > 256 {
			return nil, errors.New("merkleFunding exceeds uint256")
		}
	}
	return total, nil
}
