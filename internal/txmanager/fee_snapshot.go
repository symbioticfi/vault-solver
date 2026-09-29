package txmanager

import (
	"context"
	"math"
	"math/big"
	"slices"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// The horizon fee policy prices every attempt from a fee snapshot (strategy §2.1, §2.4): the latest header
// and one eth_feeHistory(k, latest, [25, run percentile]) read, which give the next block's exact base fee
// (computed by the node), and per block the gas used against the header's gas limit and the priority-fee
// rewards the tip rule and the legacy rule follow.
//
// Goroutine model: the worker (initial sends), quote goroutines (MaxFeePerGas), the lifecycle goroutine
// (pending evaluation, pending.go) and the shadow evaluator (shadow.go) read snapshots through
// feeSnapshotCache; a pending evaluation whose blocks the cached history does not cover reads a longer one of
// its own, uncached. The cache is the only writer:
// the one goroutine running a shared read (singleflight) stores its result under the cache mutex. A stored
// snapshot is immutable, so readers share it without copying and must not modify its fields.

// feeSnapshotBlocks is how many blocks a cached snapshot's fee history covers: the tip rule needs the latest
// two, the legacy rule's tip the latest five, and the shadow evaluator scores three blocks after a head;
// one more tolerates a skipped head. fees.congestedRewardBlocks raises it.
const feeSnapshotBlocks = 6

// feeSnapshotKey is the singleflight key of the one shared snapshot read.
const feeSnapshotKey = "fee-snapshot"

// errSnapshotInconsistent is a fee history whose newest block is more than one block from the latest
// header, read twice in a row: the two reads came from upstreams at different heads. A send treats it as a
// stale head and waits for a consistent one.
var errSnapshotInconsistent = errors.New("fee history and latest header disagree on the head")

// feeSnapshot is one read of the fee inputs of the horizon policy.
type feeSnapshot struct {
	header      *types.Header // latest header: number, hash, time and the gas limit room is measured against
	historyHead uint64        // newest block the fee history describes (fhHead)
	nextBase    *big.Int      // base fee of the block after historyHead (pb), computed by the node
	blocks      []feeBlock    // the fee history, oldest first, ending at historyHead
	readAt      time.Time
}

// feeBlock is one block of a fee snapshot's history.
type feeBlock struct {
	number       uint64
	baseFee      *big.Int
	gasUsedRatio float64
	legacyReward *big.Int // reward at the legacy rule's percentile (feeHistoryPercentile); nil when none
	runReward    *big.Int // reward at fees.congestedRewardPercentile; nil when the node reported none
}

// snapshotRewards is the reward percentiles a snapshot reads, ascending as eth_feeHistory requires, and
// the index of each rule's percentile in them.
type snapshotRewards struct {
	percentiles []float64
	legacy, run int
}

func newSnapshotRewards(runPercentile float64) snapshotRewards {
	percentiles := []float64{feeHistoryPercentile}
	if runPercentile != feeHistoryPercentile {
		percentiles = append(percentiles, runPercentile)
		slices.Sort(percentiles)
	}
	return snapshotRewards{
		percentiles: percentiles,
		legacy:      slices.Index(percentiles, feeHistoryPercentile),
		run:         slices.Index(percentiles, runPercentile),
	}
}

// lag is how many blocks the snapshot trails the real next block at now: whole block times since the
// header's timestamp, plus one when the fee history is a block behind the header, whose next base fee is
// then the header's own block's rather than the next one's.
func (s *feeSnapshot) lag(now time.Time, blockTime time.Duration) uint64 {
	return snapshotLag(s.header, s.historyHead, now, blockTime)
}

// snapshotLag is the lag of a snapshot with this latest header and fee-history head (see feeSnapshot.lag).
func snapshotLag(head *types.Header, historyHead uint64, now time.Time, blockTime time.Duration) uint64 {
	lag := headTimeLag(head.Time, now, blockTime)
	if number := head.Number.Uint64(); number > historyHead {
		lag += number - historyHead
	}
	return lag
}

// snapshotBlocks is how many blocks the cached snapshot's fee history covers.
func (m *Manager) snapshotBlocks() uint64 {
	return max(feeSnapshotBlocks, m.cfg.Fees.CongestedRewardBlocks)
}

// readFeeSnapshot reads a fee snapshot covering blocks blocks within the fee-read budget. A fee history more
// than one block from the header is read again once at once; a second disagreement is errSnapshotInconsistent.
func (m *Manager) readFeeSnapshot(ctx context.Context, blocks uint64) (*feeSnapshot, error) {
	snapshot, err := m.readFeeSnapshotOnce(ctx, blocks)
	if errors.Is(err, errSnapshotInconsistent) {
		snapshot, err = m.readFeeSnapshotOnce(ctx, blocks)
	}
	return snapshot, err
}

// readFeeSnapshotOnce reads the latest header and the fee history concurrently, both at latest: pinning
// the history to a separately read header number would fail on a lagging upstream, which answers "beyond
// head" and which eRPC does not fail over on. Their agreement is checked instead.
func (m *Manager) readFeeSnapshotOnce(ctx context.Context, blocks uint64) (*feeSnapshot, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()
	var (
		head    *types.Header
		history *ethereum.FeeHistory
	)
	group, groupCtx := errgroup.WithContext(readCtx)
	group.Go(func() error {
		var err error
		if head, err = m.backend.HeaderByNumber(groupCtx, nil); err != nil {
			return errors.Errorf("header by number: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		var err error
		if history, err = m.backend.FeeHistory(groupCtx, blocks, nil, m.snapshotRewards.percentiles); err != nil {
			return errors.Errorf("fee history: %w", err)
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, errors.Errorf("%w: %w", errFreshFeesUnavailable, err)
	}
	return newFeeSnapshot(head, history, m.snapshotRewards, time.Now())
}

// newFeeSnapshot validates one header and fee history read and builds the snapshot. Rewards may be absent
// altogether (some nodes omit them for empty blocks), which counts as zero rewards; otherwise every block
// must carry one non-negative reward per requested percentile.
func newFeeSnapshot(
	head *types.Header, history *ethereum.FeeHistory, rewards snapshotRewards, now time.Time,
) (*feeSnapshot, error) {
	if head == nil || head.Number == nil || !head.Number.IsUint64() || head.Number.Uint64() >= math.MaxInt64 {
		return nil, errors.Errorf("%w: latest header has no usable block number", errFreshFeesUnavailable)
	}
	if head.BaseFee == nil || head.BaseFee.Sign() < 0 || head.GasLimit == 0 {
		return nil, errors.Errorf("%w: latest header must carry a base fee and a gas limit", errFreshFeesUnavailable)
	}
	if history == nil || history.OldestBlock == nil || !history.OldestBlock.IsUint64() {
		return nil, errors.Errorf("%w: fee history has no oldest block", errFreshFeesUnavailable)
	}
	count := len(history.GasUsedRatio)
	if count == 0 || len(history.BaseFee) != count+1 {
		return nil, errors.Errorf(
			"%w: fee history has %d gas-used ratios and %d base fees", errFreshFeesUnavailable, count, len(history.BaseFee),
		)
	}
	if len(history.Reward) != 0 && len(history.Reward) != count {
		return nil, errors.Errorf("%w: fee history has %d reward rows for %d blocks", errFreshFeesUnavailable, len(history.Reward), count)
	}
	for _, baseFee := range history.BaseFee {
		if baseFee == nil || baseFee.Sign() < 0 {
			return nil, errors.Errorf("%w: fee history has an invalid base fee", errFreshFeesUnavailable)
		}
	}
	oldest := history.OldestBlock.Uint64()
	historyHead := oldest + uint64(count-1)
	if historyHead < oldest || historyHead >= math.MaxInt64 {
		return nil, errors.Errorf("%w: fee history block range overflows", errFreshFeesUnavailable)
	}
	number := head.Number.Uint64()
	if historyHead > number+1 || number > historyHead+1 {
		return nil, errors.Errorf("%w: fee history head %d, header %d", errSnapshotInconsistent, historyHead, number)
	}
	snapshot := &feeSnapshot{
		header:      types.CopyHeader(head),
		historyHead: historyHead,
		nextBase:    new(big.Int).Set(history.BaseFee[count]),
		blocks:      make([]feeBlock, count),
		readAt:      now,
	}
	for i := range count {
		ratio := history.GasUsedRatio[i]
		if math.IsNaN(ratio) || ratio < 0 || ratio > 1 {
			return nil, errors.Errorf("%w: fee history has gas-used ratio %v", errFreshFeesUnavailable, ratio)
		}
		block := feeBlock{number: oldest + uint64(i), baseFee: new(big.Int).Set(history.BaseFee[i]), gasUsedRatio: ratio}
		if len(history.Reward) != 0 {
			row := history.Reward[i]
			if len(row) != len(rewards.percentiles) {
				return nil, errors.Errorf("%w: fee history block %d has %d rewards for %d percentiles",
					errFreshFeesUnavailable, block.number, len(row), len(rewards.percentiles))
			}
			for _, reward := range row {
				if reward == nil || reward.Sign() < 0 {
					return nil, errors.Errorf("%w: fee history block %d has an invalid reward", errFreshFeesUnavailable, block.number)
				}
			}
			block.legacyReward = new(big.Int).Set(row[rewards.legacy])
			block.runReward = new(big.Int).Set(row[rewards.run])
		}
		snapshot.blocks[i] = block
	}
	return snapshot, nil
}

// feeSnapshotCache holds the latest fee snapshot and runs at most one read at a time, which concurrent
// callers share (singleflight), so quote pricing costs no RPCs while a recent snapshot exists. It is the
// only writer of the snapshot (see the goroutine model above).
type feeSnapshotCache struct {
	read   func(context.Context) (*feeSnapshot, error)
	flight singleflight.Group

	mu     sync.Mutex
	latest *feeSnapshot
}

// get returns the latest snapshot if it was read less than maxAge ago, and otherwise reads a new one,
// joining a read already in flight. maxAge 0 always reads (or joins). The shared read is detached from
// ctx's cancellation, since other callers may be waiting on it, and bounded by the fee-read budget; ctx
// only bounds this caller's wait.
func (c *feeSnapshotCache) get(ctx context.Context, maxAge time.Duration) (*feeSnapshot, error) {
	return c.getNewer(ctx, maxAge, time.Time{})
}

// getNewer is get for a reader that polls: it also reads (or joins a read) when the cached snapshot is not
// newer than seen, the read time of the snapshot the caller took last, so a poll never takes the snapshot it
// already has for a new one. The zero seen accepts any cached snapshot. The flight checks the cache again
// before reading: a caller that missed the cache just before a read in flight stored its snapshot, and then
// the flight just after it ended, takes that snapshot rather than starting a second read.
func (c *feeSnapshotCache) getNewer(ctx context.Context, maxAge time.Duration, seen time.Time) (*feeSnapshot, error) {
	usable := func() *feeSnapshot {
		if snapshot := c.cached(maxAge); snapshot != nil && snapshot.readAt.After(seen) {
			return snapshot
		}
		return nil
	}
	if snapshot := usable(); snapshot != nil {
		return snapshot, nil
	}
	results := c.flight.DoChan(feeSnapshotKey, func() (any, error) {
		if snapshot := usable(); snapshot != nil {
			return snapshot, nil
		}
		snapshot, err := c.read(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		c.store(snapshot)
		return snapshot, nil
	})
	select {
	case result := <-results:
		if result.Err != nil {
			return nil, result.Err
		}
		snapshot, ok := result.Val.(*feeSnapshot)
		if !ok || snapshot == nil {
			return nil, errors.Errorf("%w: fee snapshot read returned no snapshot", errFreshFeesUnavailable)
		}
		return snapshot, nil
	case <-ctx.Done():
		return nil, errors.Errorf("%w: %w", errFreshFeesUnavailable, context.Cause(ctx))
	}
}

func (c *feeSnapshotCache) cached(maxAge time.Duration) *feeSnapshot {
	if maxAge <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latest != nil && time.Since(c.latest.readAt) < maxAge {
		return c.latest
	}
	return nil
}

func (c *feeSnapshotCache) store(snapshot *feeSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest = snapshot
}

// horizonSendSnapshot is the fee snapshot a new horizon-policy attempt is priced from: its own read of the
// latest header and fee history (strategy §2.5 step 1), sharing a read already in flight but never the
// quotes' cached snapshot, which can trail the head by a block within the poll interval. A stale one is read
// again (see checkFeeSnapshot) for up to two block times within ctx, then refused with ErrStaleHead. The
// staleness rules are the balance guard's, and apply whether or not the guard runs: the exact validity
// horizon is only exact from a fresh head. onRead sees every consistent snapshot read, stale or not, before
// it is checked, so the gas estimate can start on the first one.
func (m *Manager) horizonSendSnapshot(ctx context.Context, onRead func(*feeSnapshot)) (sendSnapshot, error) {
	return m.awaitFreshSnapshot(ctx, func(ctx context.Context) (snapshot sendSnapshot, stale, err error) {
		fees, err := m.snapshots.get(ctx, 0)
		switch {
		case errors.Is(err, errSnapshotInconsistent):
			return sendSnapshot{}, err, nil
		case err != nil:
			return sendSnapshot{}, nil, err
		}
		onRead(fees)
		snapshot, stale = m.checkFeeSnapshot(fees, time.Now())
		return snapshot, stale, nil
	})
}

// checkFeeSnapshot derives the head lag and the balance pin of a horizon fee snapshot, or reports why its
// head is stale: the lag exceeds fees.maxHeadLagBlocks, or its fee history ends below the previous
// lifecycle's inclusion block.
func (m *Manager) checkFeeSnapshot(fees *feeSnapshot, now time.Time) (sendSnapshot, error) {
	lag := fees.lag(now, m.cfg.Fees.BlockTime)
	pin, err := m.snapshotPin(fees.header, fees.historyHead, lag)
	if err != nil {
		return sendSnapshot{}, err
	}
	return sendSnapshot{
		head:        fees.header.Number.Uint64(),
		historyHead: fees.historyHead,
		nextBase:    new(big.Int).Set(fees.nextBase),
		lag:         lag,
		pin:         pin,
		fees:        fees,
	}, nil
}
