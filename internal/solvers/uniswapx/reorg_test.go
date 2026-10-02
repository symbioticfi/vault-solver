package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type reorgChainReader struct {
	chainReader

	provenReorg bool
	err         error
	checks      int
	block       uint64
	hash        common.Hash
	canonical   *types.Receipt
}

func (r *reorgChainReader) reconcileFillInclusion(
	_ context.Context, receipt *types.Receipt,
) (bool, *types.Receipt, error) {
	r.checks++
	r.block, r.hash = receipt.BlockNumber.Uint64(), receipt.BlockHash
	canonical := r.canonical
	if canonical == nil && !r.provenReorg {
		canonical = receipt
	}
	return r.provenReorg, canonical, r.err
}

type reorgOrderPoller struct {
	orderPoller

	status  string
	err     error
	lookups int
}

func (p *reorgOrderPoller) ordersByHash(
	_ context.Context, _ int64, hashes []common.Hash,
) (map[common.Hash]orderTerminal, error) {
	p.lookups++
	if p.err != nil {
		return nil, p.err
	}
	terminals := make(map[common.Hash]orderTerminal, len(hashes))
	for _, hash := range hashes {
		terminals[hash] = orderTerminal{Status: p.status}
	}
	return terminals, nil
}

func TestPollSourceReopensOrphanedFillWithFreshExecution(t *testing.T) {
	fixture := newTracingFillFixture(t)
	receipt := &types.Receipt{
		BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0xb100"),
		Status: types.ReceiptStatusSuccessful, TxHash: tracingFillTxHash,
	}
	fixture.runWithResult(t, txmanager.Result{
		Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed, Receipt: receipt,
	})
	receipt.BlockNumber.SetUint64(999) // The manager's result must not mutate retained evidence.
	hash := common.HexToHash(fixture.entry.OrderHash)
	reader := &reorgChainReader{chainReader: fixture.solver.reader, provenReorg: true}
	fixture.solver.reader = reader
	fixture.solver.orders = &reorgOrderPoller{orderPoller: fixture.solver.orders, status: orderStatusOpen}
	fixture.solver.exclusiveTerminal[hash] = time.Now()
	fixture.solver.quoteState.Store(&quoteState{})
	orders := make(chan *resolvedOrder, 2)
	if _, err := fixture.solver.pollSource(
		t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 {
		t.Fatalf("reopened orders = %d, want 1 after canonical fill block changed", len(orders))
	}
	if reader.checks != 1 || reader.block != 100 || reader.hash != receipt.BlockHash {
		t.Fatalf("completion probe = %d/%d/%s", reader.checks, reader.block, reader.hash)
	}
	if _, done := fixture.solver.filled[hash]; done {
		t.Fatal("orphaned fill remained completed")
	}
	if _, terminal := fixture.solver.exclusiveTerminal[hash]; terminal {
		t.Fatal("orphaned fill retained its exclusive terminal suppression")
	}
	if _, tracked := fixture.solver.exclusiveUntil[hash]; !tracked {
		t.Fatal("reopened exclusive obligation was not tracked")
	}
	if fixture.solver.quoteState.Load() != nil || fixture.solver.capacity.Len() != 0 {
		t.Fatal("reopened fill kept cached inventory or the old reservation")
	}
	if _, err := fixture.solver.pollSource(
		t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || reader.checks != 1 {
		t.Fatal("duplicate listing admitted another fill while reopened order was in flight")
	}
	order := <-orders
	fixture.solver.endFillPlanning()
	pending, err := fixture.solver.startFill(
		t.Context(), []liquidlane.Route{fixture.route}, order, time.Now(), time.Now(),
	)
	if err != nil {
		t.Fatalf("fresh fill planning: %v", err)
	}
	if pending == nil || len(fixture.txm.reqs) != 2 {
		t.Fatalf("fresh retry pending/requests = %v/%d", pending, len(fixture.txm.reqs))
	}
	request := fixture.txm.reqs[1]
	if request.Obsolete == nil || request.CancelAt.IsZero() || request.Solver != Name {
		t.Fatal("reopened fill lost its normal request protections")
	}
	if obsolete, err := request.Obsolete(t.Context()); obsolete || err != nil {
		t.Fatalf("fresh open order obsolete = %v, error = %v", obsolete, err)
	}
	fixture.solver.completePendingFill(t.Context(), pending, txmanager.Result{
		Hash: common.HexToHash("0x3"), Outcome: txmanager.OutcomeConfirmed, Receipt: receipt,
	})
}

func TestPollSourceReconcilesObsoleteOrderByCurrentStatus(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    string
		wantQueue int
	}{
		{name: "stale open listing but still filled", status: orderStatusFilled},
		{name: "terminal observation rolled back", status: orderStatusOpen, wantQueue: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newTracingFillFixture(t)
			fixture.runWithResult(t, txmanager.Result{
				Hash: tracingFillTxHash, Outcome: txmanager.OutcomeCancelled, Err: txmanager.ErrRequestObsolete,
				Receipt: &types.Receipt{BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0xb100")},
			})
			reader := &reorgChainReader{chainReader: fixture.solver.reader}
			ordersAPI := &reorgOrderPoller{orderPoller: fixture.solver.orders, status: tc.status}
			fixture.solver.reader, fixture.solver.orders = reader, ordersAPI
			orders := make(chan *resolvedOrder, 1)
			if _, err := fixture.solver.pollSource(
				t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
			); err != nil {
				t.Fatal(err)
			}
			if len(orders) != tc.wantQueue || ordersAPI.lookups != 1 {
				t.Fatalf("obsolete order queue/lookups = %d/%d, want %d/1", len(orders), ordersAPI.lookups, tc.wantQueue)
			}
			if reader.checks != 0 {
				t.Fatal("cancellation receipt was mistaken for an owned fill receipt")
			}
		})
	}
}

func TestPollSourceDoesNotReopenIncludedFillWithMissingReceiptMetadata(t *testing.T) {
	for _, receipt := range []*types.Receipt{
		nil,
		{},
		{BlockNumber: big.NewInt(100)},
		{BlockHash: common.HexToHash("0xb100")},
		{BlockNumber: big.NewInt(-1), BlockHash: common.HexToHash("0xb100")},
	} {
		fixture := newTracingFillFixture(t)
		fixture.runWithResult(t, txmanager.Result{
			Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed, Receipt: receipt,
		})
		reader := &reorgChainReader{chainReader: fixture.solver.reader, provenReorg: true}
		ordersAPI := &reorgOrderPoller{orderPoller: fixture.solver.orders, status: orderStatusOpen}
		fixture.solver.reader, fixture.solver.orders = reader, ordersAPI
		orders := make(chan *resolvedOrder, 1)
		if _, err := fixture.solver.pollSource(
			t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
		); err == nil {
			t.Fatal("included fill with incomplete canonical evidence did not fail closed")
		}
		if len(orders) != 0 || reader.checks != 0 || ordersAPI.lookups != 0 {
			t.Fatal("missing fill evidence was treated as proof of a reorg")
		}
	}
}

func TestPollSourceContinuesAfterCompletedOrderReconciliationError(t *testing.T) {
	fixture := newTracingFillFixture(t)
	fixture.runWithResult(t, txmanager.Result{
		Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed,
		Receipt: &types.Receipt{BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0xb100")},
	})
	originalReader := fixture.solver.reader.(*executionTestReader)
	reader := &reorgChainReader{chainReader: originalReader, err: errors.New("receipt block RPC unavailable")}
	fixture.solver.reader = reader
	second := tracingOrderEntry(
		t, fixture.solver.cfg, fixture.route.TokenIn, fixture.route.TokenOut,
		originalReader.now.Add(time.Second),
	)
	fixture.solver.orders = orderPollerFunc(func(context.Context, int64, *common.Address) ([]orderEntry, error) {
		return []orderEntry{fixture.entry, second}, nil
	})
	orders := make(chan *resolvedOrder, 2)
	if _, err := fixture.solver.pollSource(
		t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
	); err == nil {
		t.Fatal("completion read error was swallowed")
	}
	if len(orders) != 1 || (<-orders).Hash != common.HexToHash(second.OrderHash) {
		t.Fatal("one completion read error prevented unrelated open order admission")
	}
}

func TestPollSourceRetainsCanonicalReinclusionBeforeLaterReorg(t *testing.T) {
	fixture := newTracingFillFixture(t)
	fixture.runWithResult(t, txmanager.Result{
		Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed,
		Receipt: &types.Receipt{
			TxHash: tracingFillTxHash, Status: types.ReceiptStatusSuccessful,
			BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0xb100"),
		},
	})
	reincluded := &types.Receipt{
		TxHash: tracingFillTxHash, Status: types.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(101), BlockHash: common.HexToHash("0xb101"),
	}
	reader := &reorgChainReader{chainReader: fixture.solver.reader, canonical: reincluded}
	ordersAPI := &reorgOrderPoller{orderPoller: fixture.solver.orders, status: orderStatusOpen}
	fixture.solver.reader, fixture.solver.orders = reader, ordersAPI
	orders := make(chan *resolvedOrder, 1)
	if _, err := fixture.solver.pollSource(
		t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 || ordersAPI.lookups != 0 {
		t.Fatal("same transaction re-included canonically was retried from stale open API state")
	}
	hash := common.HexToHash(fixture.entry.OrderHash)
	if proof := fixture.solver.completedBlocks[hash]; proof.BlockNumber.Uint64() != 101 || proof.BlockHash != reincluded.BlockHash {
		t.Fatalf("fresh inclusion evidence was not adopted: %v", proof)
	}
	reincluded.BlockNumber.SetUint64(999)
	reader.canonical, reader.provenReorg = nil, true
	if _, err := fixture.solver.pollSource(
		t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatal(err)
	}
	if reader.block != 101 || reader.hash != common.HexToHash("0xb101") || len(orders) != 1 {
		t.Fatal("later reorg did not reconcile the retained new inclusion block")
	}
}

func TestPollSourcePreservesCompletedFillWithoutReorgAndOpenProof(t *testing.T) {
	tests := []struct {
		name        string
		provenReorg bool
		probeErr    error
		status      string
		statusErr   error
		wantErr     bool
		wantLookup  bool
	}{
		{name: "canonical fill and stale open listing", status: orderStatusOpen},
		{name: "receipt RPC failure", probeErr: errors.New("RPC unavailable"), status: orderStatusOpen, wantErr: true},
		{name: "orphan but new fill", provenReorg: true, status: orderStatusFilled, wantLookup: true},
		{name: "orphan but cancelled", provenReorg: true, status: orderStatusCancelled, wantLookup: true},
		{name: "orphan and status failure", provenReorg: true, statusErr: errors.New("API unavailable"), wantErr: true, wantLookup: true},
		{name: "orphan and unknown status", provenReorg: true, status: "renamed-status", wantErr: true, wantLookup: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newTracingFillFixture(t)
			fixture.runWithResult(t, txmanager.Result{
				Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed,
				Receipt: &types.Receipt{BlockNumber: big.NewInt(100), BlockHash: common.HexToHash("0xb100")},
			})
			hash := common.HexToHash(fixture.entry.OrderHash)
			reader := &reorgChainReader{chainReader: fixture.solver.reader, provenReorg: tc.provenReorg, err: tc.probeErr}
			ordersAPI := &reorgOrderPoller{orderPoller: fixture.solver.orders, status: tc.status, err: tc.statusErr}
			fixture.solver.reader, fixture.solver.orders = reader, ordersAPI
			orders := make(chan *resolvedOrder, 1)
			_, err := fixture.solver.pollSource(t.Context(), orderSourceExclusiveV2, &fixture.solver.cfg.Executor, orders)
			if (err != nil) != tc.wantErr {
				t.Fatalf("poll error = %v, want error = %v", err, tc.wantErr)
			}
			if len(orders) != 0 || fixture.solver.inFlight[hash] {
				t.Fatal("unproven open order was admitted for another fill")
			}
			if _, done := fixture.solver.filled[hash]; !done {
				t.Fatal("completed suppression cleared without proven reorg and fresh open status")
			}
			if got := ordersAPI.lookups > 0; got != tc.wantLookup {
				t.Fatalf("status lookup = %v, want %v", got, tc.wantLookup)
			}
		})
	}
}
