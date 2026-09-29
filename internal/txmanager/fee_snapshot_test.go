package txmanager

import (
	"context"
	"errors"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestNewSnapshotRewards(t *testing.T) {
	for _, tc := range []struct {
		run         float64
		want        []float64
		legacy, idx int
	}{
		{run: 50, want: []float64{25, 50}, legacy: 0, idx: 1},
		{run: 25, want: []float64{25}, legacy: 0, idx: 0},
		{run: 10, want: []float64{10, 25}, legacy: 1, idx: 0},
	} {
		got := newSnapshotRewards(tc.run)
		if len(got.percentiles) != len(tc.want) || got.legacy != tc.legacy || got.run != tc.idx {
			t.Fatalf("newSnapshotRewards(%v) = %+v, want percentiles %v, legacy %d, run %d", tc.run, got, tc.want, tc.legacy, tc.idx)
		}
		for i := range tc.want {
			if got.percentiles[i] != tc.want[i] {
				t.Fatalf("newSnapshotRewards(%v) percentiles = %v, want %v", tc.run, got.percentiles, tc.want)
			}
		}
	}
}

func TestNewFeeSnapshot(t *testing.T) {
	rewards := newSnapshotRewards(50)
	header := func() *types.Header {
		return &types.Header{Number: big.NewInt(100), GasLimit: 36_000_000, BaseFee: big.NewInt(1e9), Time: 1_700_000_000}
	}
	// history is a fee history ending at newest, of blocks blocks with p25 = 1 wei and p50 = 2 wei.
	history := func(newest uint64, blocks int) *ethereum.FeeHistory {
		h := &ethereum.FeeHistory{OldestBlock: new(big.Int).SetUint64(newest - uint64(blocks) + 1)}
		for range blocks {
			h.BaseFee = append(h.BaseFee, big.NewInt(1e9))
			h.GasUsedRatio = append(h.GasUsedRatio, 0.5)
			h.Reward = append(h.Reward, []*big.Int{big.NewInt(1), big.NewInt(2)})
		}
		h.BaseFee = append(h.BaseFee, big.NewInt(1_100_000_000))
		return h
	}
	for _, tc := range []struct {
		name    string
		header  func(h *types.Header)
		history func(h *ethereum.FeeHistory) *ethereum.FeeHistory
		newest  uint64
		wantErr error
	}{
		{name: "consistent", newest: 100},
		{name: "one block behind", newest: 99},
		{name: "one block ahead", newest: 101},
		{name: "two blocks behind", newest: 98, wantErr: errSnapshotInconsistent},
		{name: "two blocks ahead", newest: 102, wantErr: errSnapshotInconsistent},
		{
			name: "no rewards count as zero", newest: 100,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.Reward = nil; return h },
		},
		{name: "header without a gas limit", newest: 100, header: func(h *types.Header) { h.GasLimit = 0 }, wantErr: errFreshFeesUnavailable},
		{name: "header without a base fee", newest: 100, header: func(h *types.Header) { h.BaseFee = nil }, wantErr: errFreshFeesUnavailable},
		{name: "header without a number", newest: 100, header: func(h *types.Header) { h.Number = nil }, wantErr: errFreshFeesUnavailable},
		{
			name: "no oldest block", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.OldestBlock = nil; return h },
		},
		{
			name: "no next base fee", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.BaseFee = h.BaseFee[:len(h.BaseFee)-1]; return h },
		},
		{
			name: "a gas-used ratio above one", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.GasUsedRatio[1] = 1.2; return h },
		},
		{
			name: "a NaN gas-used ratio", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.GasUsedRatio[1] = math.NaN(); return h },
		},
		{
			name: "a short reward row", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.Reward[2] = h.Reward[2][:1]; return h },
		},
		{
			name: "a missing reward row", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.Reward = h.Reward[1:]; return h },
		},
		{
			name: "a negative reward", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.Reward[0][1] = big.NewInt(-1); return h },
		},
		{
			name: "a nil base fee", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(h *ethereum.FeeHistory) *ethereum.FeeHistory { h.BaseFee[0] = nil; return h },
		},
		{
			name: "no blocks", newest: 100, wantErr: errFreshFeesUnavailable,
			history: func(*ethereum.FeeHistory) *ethereum.FeeHistory {
				return &ethereum.FeeHistory{OldestBlock: big.NewInt(100), BaseFee: []*big.Int{big.NewInt(1)}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := header()
			if tc.header != nil {
				tc.header(head)
			}
			fh := history(tc.newest, 6)
			if tc.history != nil {
				fh = tc.history(fh)
			}
			snapshot, err := newFeeSnapshot(head, fh, rewards, time.Unix(1_700_000_000, 0))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || snapshot != nil {
					t.Fatalf("newFeeSnapshot = (%v, %v), want %v", snapshot, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newFeeSnapshot: %v", err)
			}
			if snapshot.historyHead != tc.newest || snapshot.nextBase.Cmp(big.NewInt(1_100_000_000)) != 0 || len(snapshot.blocks) != 6 {
				t.Fatalf("snapshot head %d next base %s blocks %d, want %d, 1.1 gwei, 6",
					snapshot.historyHead, snapshot.nextBase, len(snapshot.blocks), tc.newest)
			}
			last := snapshot.blocks[len(snapshot.blocks)-1]
			if last.number != tc.newest {
				t.Fatalf("newest block = %d, want %d", last.number, tc.newest)
			}
			if fh.Reward != nil && (last.legacyReward.Cmp(big.NewInt(1)) != 0 || last.runReward.Cmp(big.NewInt(2)) != 0) {
				t.Fatalf("rewards = p25 %s p50 %s, want 1 and 2", last.legacyReward, last.runReward)
			}
			if fh.Reward == nil && (last.legacyReward != nil || last.runReward != nil) {
				t.Fatalf("rewards without a reward row = %s / %s, want none", last.legacyReward, last.runReward)
			}
			// The snapshot owns its values: the node's response can be reused without changing it.
			fh.BaseFee[len(fh.BaseFee)-1].SetInt64(7)
			head.Number.SetInt64(7)
			if snapshot.nextBase.Int64() == 7 || snapshot.header.Number.Int64() == 7 {
				t.Fatal("the snapshot aliases the response it was built from")
			}
		})
	}
}

func TestFeeSnapshotLag(t *testing.T) {
	now := time.Unix(1_700_000_100, 0)
	for _, tc := range []struct {
		name        string
		headTime    uint64
		historyHead uint64
		want        uint64
	}{
		{name: "fresh", headTime: 1_700_000_095, historyHead: 100},
		{name: "one block time old", headTime: 1_700_000_088, historyHead: 100, want: 1},
		{name: "history one block behind", headTime: 1_700_000_095, historyHead: 99, want: 1},
		{name: "history ahead is not charged", headTime: 1_700_000_095, historyHead: 101},
		{name: "both", headTime: 1_700_000_070, historyHead: 99, want: 3},
	} {
		snapshot := &feeSnapshot{header: &types.Header{Number: big.NewInt(100), Time: tc.headTime}, historyHead: tc.historyHead}
		if got := snapshot.lag(now, 12*time.Second); got != tc.want {
			t.Fatalf("%s: lag = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestReadFeeSnapshotRetriesADisagreementOnce(t *testing.T) {
	for _, tc := range []struct {
		name      string
		offsets   []int64
		wantReads int
		wantErr   error
	}{
		{name: "agrees", offsets: []int64{0}, wantReads: 1},
		{name: "agrees on the retry", offsets: []int64{-2, 0}, wantReads: 2},
		{name: "disagrees twice", offsets: []int64{3, -2}, wantReads: 2, wantErr: errSnapshotInconsistent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newHorizonBackend(big.NewInt(1e18))
			b.historyOffset = tc.offsets
			m := newHorizonManager(t, b, horizonConfig(), nil)
			snapshot, err := m.readFeeSnapshot(t.Context(), feeSnapshotBlocks)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil) != (snapshot != nil) {
				t.Fatalf("readFeeSnapshot = (%v, %v), want error %v", snapshot, err, tc.wantErr)
			}
			if reads := b.feeHistoryReads(); len(reads) != tc.wantReads {
				t.Fatalf("fee history read %d times, want %d", len(reads), tc.wantReads)
			}
		})
	}
}

func TestFeeSnapshotCache(t *testing.T) {
	snapshotAt := func(head uint64) *feeSnapshot {
		return &feeSnapshot{header: &types.Header{Number: new(big.Int).SetUint64(head)}, historyHead: head, readAt: time.Now()}
	}
	t.Run("concurrent callers share one read", func(t *testing.T) {
		var reads atomic.Int64
		release := make(chan struct{})
		cache := &feeSnapshotCache{read: func(context.Context) (*feeSnapshot, error) {
			reads.Add(1)
			<-release
			return snapshotAt(100), nil
		}}
		var wg sync.WaitGroup
		results := make(chan *feeSnapshot, 8)
		for range 8 {
			wg.Go(func() {
				snapshot, err := cache.get(t.Context(), time.Hour)
				if err != nil {
					t.Errorf("get: %v", err)
				}
				results <- snapshot
			})
		}
		deadline := time.Now().Add(5 * time.Second)
		for reads.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		close(release)
		wg.Wait()
		close(results)
		for snapshot := range results {
			if snapshot == nil || snapshot.historyHead != 100 {
				t.Fatalf("a caller got %v, want the shared snapshot", snapshot)
			}
		}
		if got := reads.Load(); got != 1 {
			t.Fatalf("eight concurrent callers ran %d reads, want 1", got)
		}
		if _, err := cache.get(t.Context(), time.Hour); err != nil || reads.Load() != 1 {
			t.Fatalf("a cached snapshot was read again (%d reads, %v)", reads.Load(), err)
		}
		if _, err := cache.get(t.Context(), 0); err != nil || reads.Load() != 2 {
			t.Fatalf("maxAge 0 did not read (%d reads, %v)", reads.Load(), err)
		}
	})
	t.Run("an abandoned caller does not end the shared read", func(t *testing.T) {
		release := make(chan struct{})
		var readErr atomic.Value
		cache := &feeSnapshotCache{read: func(ctx context.Context) (*feeSnapshot, error) {
			<-release
			if err := ctx.Err(); err != nil {
				readErr.Store(err)
			}
			return snapshotAt(101), nil
		}}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := cache.get(ctx, 0)
			done <- err
		}()
		cancel()
		if err := <-done; !errors.Is(err, errFreshFeesUnavailable) || !errors.Is(err, context.Canceled) {
			t.Fatalf("abandoned get = %v, want a fresh-fees error wrapping the cancellation", err)
		}
		close(release)
		snapshot, err := cache.get(t.Context(), 0)
		if err != nil || snapshot.historyHead != 101 || readErr.Load() != nil {
			t.Fatalf("shared read after an abandoned caller = (%v, %v, read context %v)", snapshot, err, readErr.Load())
		}
	})
	t.Run("a failed read is not cached", func(t *testing.T) {
		var fail atomic.Bool
		fail.Store(true)
		cache := &feeSnapshotCache{read: func(context.Context) (*feeSnapshot, error) {
			if fail.Load() {
				return nil, errFreshFeesUnavailable
			}
			return snapshotAt(102), nil
		}}
		if _, err := cache.get(t.Context(), time.Hour); !errors.Is(err, errFreshFeesUnavailable) {
			t.Fatalf("get = %v, want the read error", err)
		}
		fail.Store(false)
		if snapshot, err := cache.get(t.Context(), time.Hour); err != nil || snapshot.historyHead != 102 {
			t.Fatalf("get after a failed read = (%v, %v), want a new read", snapshot, err)
		}
	})
}
