package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/morpho"
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
) *Strategy {
	mon := &apiMonitor{log: log}
	mon.snap.Store(snapshotFromSeed(seed))
	return &Strategy{
		cfg:           cfg,
		adapter:       adapter,
		callback:      callback,
		gasAccounting: gasAccounting,
		signer:        signer,
		mon:           mon,
		engine:        newBundleEngine(cfg, log),
		log:           log,
	}
}

func (s *Strategy) StoreDecisionStateForTest(callbackNative *big.Int, updatedAt time.Time) {
	s.state.store(decisionState{
		CallbackNative:    callbackNative,
		CallbackUpdatedAt: updatedAt,
	})
}

func (s *Strategy) SnapshotForTest() SnapshotSeed {
	return seedFromSnapshot(s.mon.snapshot())
}

func (s *Strategy) StoreSnapshotForTest(seed SnapshotSeed) {
	if mon, ok := s.mon.(*apiMonitor); ok {
		mon.snap.Store(snapshotFromSeed(seed))
	}
}

func snapshotFromSeed(seed SnapshotSeed) *snapshot {
	return &snapshot{
		markets:   seed.Markets,
		prices:    seed.Prices,
		positions: seed.Positions,
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
		Markets:   snap.markets,
		Prices:    snap.prices,
		Positions: snap.positions,
		Block:     snap.block,
		BlockTime: snap.blockTime,
		UpdatedAt: snap.updatedAt,
	}
}
