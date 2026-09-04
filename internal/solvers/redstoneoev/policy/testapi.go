package policy

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

type SnapshotSeed struct {
	Markets   map[common.Hash]MarketInfo
	Prices    map[common.Hash]*big.Int
	Positions map[common.Hash]map[common.Address]morpho.PositionState
	Block     uint64
	BlockTime uint64
	UpdatedAt time.Time
}

func NewWithSnapshotForTest(
	cfg Config,
	adapter common.Address,
	callback common.Address,
	gasAccounting bool,
	seed SnapshotSeed,
	log logr.Logger,
	signer signer,
) (*Strategy, decision.FactSource) {
	mon := &apiMonitor{log: log}
	mon.snap.Store(snapshotFromSeed(seed))
	return &Strategy{
		cfg:           cfg,
		adapter:       adapter,
		callback:      callback,
		gasAccounting: gasAccounting,
		signer:        signer,
		engine:        newBundleEngine(cfg, log),
		log:           log,
	}, mon
}

func SnapshotForTest(source decision.FactSource) SnapshotSeed {
	mon, ok := source.(*apiMonitor)
	if !ok {
		return SnapshotSeed{}
	}
	return seedFromSnapshot(mon.snapshot())
}

func StoreSnapshotForTest(source decision.FactSource, seed SnapshotSeed) {
	if mon, ok := source.(*apiMonitor); ok {
		mon.snap.Store(snapshotFromSeed(seed))
	}
}

func snapshotFromSeed(seed SnapshotSeed) *snapshot {
	return &snapshot{
		markets:   cloneSeedMarkets(seed.Markets),
		prices:    cloneSeedPrices(seed.Prices),
		positions: cloneSeedPositions(seed.Positions),
		block:     seed.Block,
		blockTime: seed.BlockTime,
		updatedAt: seed.UpdatedAt,
	}
}

func seedFromSnapshot(snap *snapshot) SnapshotSeed {
	if snap == nil {
		return SnapshotSeed{}
	}
	return SnapshotSeed{
		Markets:   cloneSeedMarkets(snap.markets),
		Prices:    cloneSeedPrices(snap.prices),
		Positions: cloneSeedPositions(snap.positions),
		Block:     snap.block,
		BlockTime: snap.blockTime,
		UpdatedAt: snap.updatedAt,
	}
}

func cloneSeedMarkets(in map[common.Hash]MarketInfo) map[common.Hash]MarketInfo {
	out := make(map[common.Hash]MarketInfo, len(in))
	for id, market := range in {
		out[id] = cloneMarketInfo(market)
	}
	return out
}

func cloneSeedPrices(in map[common.Hash]*big.Int) map[common.Hash]*big.Int {
	out := make(map[common.Hash]*big.Int, len(in))
	for id, price := range in {
		out[id] = cloneBig(price)
	}
	return out
}

func cloneSeedPositions(in map[common.Hash]map[common.Address]morpho.PositionState) map[common.Hash]map[common.Address]morpho.PositionState {
	out := make(map[common.Hash]map[common.Address]morpho.PositionState, len(in))
	for id, positions := range in {
		cloned := make(map[common.Address]morpho.PositionState, len(positions))
		for borrower, position := range positions {
			cloned[borrower] = morpho.ClonePositionState(position)
		}
		out[id] = cloned
	}
	return out
}
