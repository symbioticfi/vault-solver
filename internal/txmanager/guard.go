package txmanager

import (
	"context"
	"math"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// The balance guard (strategy §2.4, docs/TXMANAGER-PLAN.md §4) runs under both fee policies. Before an
// attempt is signed it reads the signer balance pinned to the fee snapshot's head, caps the fee cap at
// what that balance funds, and refuses an attempt it cannot keep valid for fees.minHorizonBlocks blocks
// from a fresh head. A relay accepts an unfundable transaction without an error and never lands it, so
// signing one silently holds the nonce lane until its deadline; the guard never signs one.
//
// Goroutine model: the worker goroutine runs the initial-send guard and the lifecycle goroutine runs
// the replacement cap; the lifecycle slot keeps them from overlapping. The only state they share is
// Manager.lastInclusion, an atomic the lifecycle goroutine raises when a lifecycle ends in a receipt,
// and Manager.hashPinMisses, an atomic count both update as their pinned balance reads end.

// ErrUnaffordable reports a request the signer balance cannot keep valid for fees.minHorizonBlocks
// blocks from a fresh head. Nothing was signed and the nonce was not consumed; the Result is
// NotAdmitted. Funding the signer is the remedy.
var ErrUnaffordable = errors.New("signer balance cannot fund the transaction")

// ErrStaleHead reports a request refused because the fee snapshot stayed too far behind the chain
// head, or the signer balance could not be read at the snapshot's block, for longer than the guard
// waits. Nothing was signed and the nonce was not consumed; the Result is NotAdmitted. It is
// transient: the next request is priced from a fresh read.
var ErrStaleHead = errors.New("fee snapshot head is stale")

// errUnaffordableOneBlock is ErrUnaffordable where the balance still funds the next block at the floor
// tip, so the attempt might have landed in one block. It is counted apart so a funded lane that keeps
// hitting it can revisit fees.minHorizonBlocks.
var errUnaffordableOneBlock = errors.Errorf("%w for more than one block", ErrUnaffordable)

// pinnedReadRetryDelay spaces the retries of a pinned balance read whose block the node does not
// have yet, inside the fee-read budget.
const pinnedReadRetryDelay = 200 * time.Millisecond

// hashPinNotFoundErrorAfter is how many balance reads pinned by header hash in a row may end not found
// before the streak is logged at error level. One such read is a lagging upstream or a reorg; a run of
// them is either an upstream that serves heads it cannot serve state for, or a header hash computed
// locally that the node does not know: ethclient drops the node's hash and go-ethereum rehashes the
// header fields it knows, so a header field it does not know (a later fork, a non-standard chain)
// yields a hash no node has, and every guarded send would be refused as stale_head.
const hashPinNotFoundErrorAfter = 3

// pinnedBalanceBackend reads an account balance pinned to one block, by hash (EIP-1898) or number,
// through the read endpoints. *chain.Client provides it; a backend without it runs with the guard off.
type pinnedBalanceBackend interface {
	ReadBalanceAtBlock(ctx context.Context, account common.Address, block rpc.BlockNumberOrHash) (*big.Int, error)
}

// feeReading is one read of the inputs of the legacy fee rule.
type feeReading struct {
	head    *types.Header
	tip     *big.Int             // the rule's tip before it is clamped under the fee cap
	history *ethereum.FeeHistory // rewards the tip came from; nil with a positive tipGwei
}

// sendSnapshot is the fee reading a new attempt is priced from. Under the balance guard it also carries
// the next block's base fee, how far the snapshot trails the real next block, and the block the signer
// balance is read at.
type sendSnapshot struct {
	reading     feeReading
	head        uint64   // header number; zero when the header carried none
	historyHead uint64   // newest block the fee history describes, where the balance is pinned
	nextBase    *big.Int // nil when the guard is off
	lag         uint64
	pin         rpc.BlockNumberOrHash
}

func (s sendSnapshot) guarded() bool {
	return s.nextBase != nil
}

// NotAdmittedReason is the bounded reason a NotAdmitted result's error was refused for, as
// admission_rejections_total counts it: unaffordable, unaffordable_one_block and stale_head from the
// balance guard, or nonce_conflict, manager_stopped, deadline_exceeded, caller_cancelled and other before
// the worker lifecycle. Solvers put it on their declined span event and expected-skip log line.
func NotAdmittedReason(err error) string {
	if reason, ok := guardRefusalReason(err); ok {
		return string(reason)
	}
	return string(classifyAdmissionRejection(err))
}

// guardRefusalReason classifies a pre-signing refusal of the balance guard for metrics, spans and logs.
func guardRefusalReason(err error) (admissionRejectionReason, bool) {
	switch {
	case errors.Is(err, errUnaffordableOneBlock):
		return admissionRejectionUnaffordableOneBlock, true
	case errors.Is(err, ErrUnaffordable):
		return admissionRejectionUnaffordable, true
	case errors.Is(err, ErrStaleHead):
		return admissionRejectionStaleHead, true
	default:
		return "", false
	}
}

// guardEnabled reports whether the balance guard runs: it is configured on and the backend can read a
// pinned balance.
func (m *Manager) guardEnabled() bool {
	return m.balances != nil
}

// sendSnapshot reads the fees a new attempt is priced from. Without the guard that is one read, exactly
// as the legacy policy always did; with it the read is repeated until its head is fresh.
func (m *Manager) sendSnapshot(ctx context.Context) (sendSnapshot, error) {
	if !m.guardEnabled() {
		reading, err := m.readFees(ctx)
		if err != nil {
			return sendSnapshot{}, err
		}
		return sendSnapshot{reading: reading, head: headerNumber(reading.head)}, nil
	}
	return m.freshSnapshot(ctx)
}

// freshSnapshot reads the fee inputs until their head is fresh (see checkSnapshot). A stale head is
// waited out for up to two block times, bounded by ctx and so by CancelAt, and then refused with
// ErrStaleHead: an eRPC hiccup or a couple of missed slots should not become a failed order, while a
// one-block send from a stale head is the silent drop the guard exists to prevent.
func (m *Manager) freshSnapshot(ctx context.Context) (sendSnapshot, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 2*m.cfg.Fees.BlockTime)
	defer cancel()
	poll := minPositiveDuration(m.cfg.Fees.BlockTime/4, m.cfg.PollInterval)
	refuse := func(stale error) error {
		return errors.Errorf("%w after waiting up to %s: %w", ErrStaleHead, 2*m.cfg.Fees.BlockTime, stale)
	}
	var stale error
	for {
		reading, err := m.readFees(ctx)
		if err != nil {
			// A read the deadline cut short while waiting out a stale head is still that stale head.
			if stale != nil && waitCtx.Err() != nil {
				return sendSnapshot{}, refuse(stale)
			}
			return sendSnapshot{}, err
		}
		var snapshot sendSnapshot
		if snapshot, stale = m.checkSnapshot(reading, time.Now()); stale == nil {
			return snapshot, nil
		}
		observability.Log(ctx).V(1).Info("fee snapshot is stale; waiting for a newer head", "reason", stale.Error())
		if err := sleepContext(waitCtx, poll); err != nil {
			return sendSnapshot{}, refuse(stale)
		}
	}
}

// checkSnapshot derives the next base fee, the head lag and the balance pin from one fee reading, or
// reports why its head is stale. The next base fee is the one the fee history reports for the block
// after its newest; without a usable history (a positive tipGwei reads none) it is the protocol
// maximum after the header, grow(base, 1). The snapshot is stale when the history is more than one
// block from the header, when it trails the real next block by more than fees.maxHeadLagBlocks (wall
// clock since the header, plus a history one block behind), or when its block is below the previous
// lifecycle's inclusion block, whose balance may predate that fill's payment.
func (m *Manager) checkSnapshot(reading feeReading, now time.Time) (sendSnapshot, error) {
	head := reading.head
	if head.Number == nil || !head.Number.IsUint64() || head.Number.Uint64() >= math.MaxInt64 {
		return sendSnapshot{}, errors.New("latest header has no usable block number")
	}
	number := head.Number.Uint64()
	historyHead, nextBase, ok := feeHistoryNext(reading.history)
	if !ok {
		historyHead, nextBase = number, grow(head.BaseFee, 1)
	}
	if historyHead > number+1 || number > historyHead+1 {
		return sendSnapshot{}, errors.Errorf("fee history head %d is more than one block from header %d", historyHead, number)
	}
	lag := headTimeLag(head.Time, now, m.cfg.Fees.BlockTime)
	if number > historyHead {
		lag += number - historyHead
	}
	if maxLag := m.maxHeadLagBlocks(); lag > maxLag {
		return sendSnapshot{}, errors.Errorf(
			"head %d trails the next block by %d blocks, more than fees.maxHeadLagBlocks %d", number, lag, maxLag,
		)
	}
	if last := m.lastInclusion.Load(); historyHead < last {
		return sendSnapshot{}, errors.Errorf("head %d is below the previous inclusion block %d", historyHead, last)
	}
	pin := rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(int64(historyHead)))
	if historyHead == number {
		pin = rpc.BlockNumberOrHashWithHash(head.Hash(), true)
	}
	return sendSnapshot{
		reading: reading, head: number, historyHead: historyHead, nextBase: nextBase, lag: lag, pin: pin,
	}, nil
}

// feeHistoryNext returns the newest block a fee history describes and the base fee it reports for the
// block after it. eth_feeHistory returns one more base fee than blocks: the last is the next block's,
// computed by the node, so it is exact on any EIP-1559 chain.
func feeHistoryNext(history *ethereum.FeeHistory) (uint64, *big.Int, bool) {
	if history == nil || history.OldestBlock == nil || !history.OldestBlock.IsUint64() || len(history.BaseFee) < 2 {
		return 0, nil, false
	}
	next := history.BaseFee[len(history.BaseFee)-1]
	if next == nil || next.Sign() < 0 {
		return 0, nil, false
	}
	oldest := history.OldestBlock.Uint64()
	newest := oldest + uint64(len(history.BaseFee)-2)
	if newest < oldest {
		return 0, nil, false
	}
	return newest, new(big.Int).Set(next), true
}

// headTimeLag is how many whole block times have passed since the header's timestamp; a header from the
// future (clock skew) counts as current.
func headTimeLag(headTime uint64, now time.Time, blockTime time.Duration) uint64 {
	if blockTime <= 0 || headTime > math.MaxInt64 {
		return 0
	}
	elapsed := now.Sub(time.Unix(int64(headTime), 0))
	if elapsed <= 0 {
		return 0
	}
	return uint64(elapsed / blockTime)
}

func (m *Manager) maxHeadLagBlocks() uint64 {
	if m.cfg.Fees.MaxHeadLagBlocks == nil {
		return defaultMaxHeadLagBlocks
	}
	return *m.cfg.Fees.MaxHeadLagBlocks
}

// floorTip is the tip the refusal floor assumes: fees.tipFloorGwei, or a larger mandatory tipGwei,
// since a legacy attempt never signs below that.
func (m *Manager) floorTip() *big.Int {
	return maxBigCopy(gweiToWei(m.cfg.Fees.TipFloorGwei), gweiToWei(m.cfg.TipGwei))
}

// pinnedBalance reads the signer balance at pin. A node that does not have the block yet (or, for a
// hash pin, no longer has it on its canonical chain) is retried within the fee-read budget; any failure
// refuses the attempt with ErrStaleHead. It never falls back to latest or to the telemetry snapshot: a
// lagging upstream could then answer with the balance from before the previous fill was paid.
func (m *Manager) pinnedBalance(ctx context.Context, pin rpc.BlockNumberOrHash) (*big.Int, error) {
	readCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	defer cancel()
	retry := minPositiveDuration(pinnedReadRetryDelay, m.cfg.PollInterval)
	for {
		balance, err := m.balances.ReadBalanceAtBlock(readCtx, m.signer.Address(), pin)
		switch {
		case err == nil && balance != nil && balance.Sign() >= 0:
			m.hashPinFound(ctx, pin)
			return balance, nil
		case err == nil:
			return nil, errors.Errorf("%w: invalid signer balance %v at block %s", ErrStaleHead, balance, pin.String())
		case !errors.Is(err, ethereum.NotFound):
			return nil, errors.Errorf("%w: signer balance at block %s: %w", ErrStaleHead, pin.String(), err)
		}
		if sleepContext(readCtx, retry) != nil {
			err = errors.Errorf(
				"%w: signer balance at block %s not found within %s: %w", ErrStaleHead, pin.String(), m.feeReadTimeout(), err,
			)
			// A read the caller abandoned says nothing about the node.
			if ctx.Err() == nil {
				m.hashPinNotFound(ctx, pin, err)
			}
			return nil, err
		}
	}
}

// hashPinNotFound counts a balance read pinned by header hash that ended not found, and logs the
// streak at error level once, when it reaches hashPinNotFoundErrorAfter: a refusal is otherwise only an
// Info line and a stale_head count, which cannot tell this apart from a briefly lagging upstream.
func (m *Manager) hashPinNotFound(ctx context.Context, pin rpc.BlockNumberOrHash, err error) {
	if _, byHash := pin.Hash(); !byHash {
		return
	}
	if misses := m.hashPinMisses.Add(1); misses == hashPinNotFoundErrorAfter {
		observability.Log(ctx).Error(err, "balance reads pinned by header hash keep finding no block",
			"consecutiveMisses", misses,
			"hint", "if the node serves this block by number, the go-ethereum this build uses does not hash "+
				"this chain's header fields; refusals continue as stale_head until it does",
		)
	}
}

// hashPinFound ends a streak of hash-pinned balance reads that found no block.
func (m *Manager) hashPinFound(ctx context.Context, pin rpc.BlockNumberOrHash) {
	if _, byHash := pin.Hash(); !byHash {
		return
	}
	if misses := m.hashPinMisses.Swap(0); misses >= hashPinNotFoundErrorAfter {
		observability.Log(ctx).Info("balance reads pinned by header hash recovered", "consecutiveMisses", misses)
	}
}

// replacementBalanceCap is the largest fee cap the signer balance funds for a same-nonce replacement
// with this gas limit and value, read at the current head pinned by hash as for a new send. The pending
// attempt is not mined (the replacement nonce check ran first), so the balance still holds its funds.
func (m *Manager) replacementBalanceCap(ctx context.Context, gas uint64, value *big.Int) (*big.Int, error) {
	headCtx, cancel := context.WithTimeout(ctx, m.feeReadTimeout())
	head, err := m.backend.HeaderByNumber(headCtx, nil)
	cancel()
	if err != nil {
		return nil, errors.Errorf("%w: header by number: %w", ErrStaleHead, err)
	}
	if head == nil || head.Number == nil || !head.Number.IsUint64() {
		return nil, errors.Errorf("%w: latest header has no usable block number", ErrStaleHead)
	}
	if last := m.lastInclusion.Load(); head.Number.Uint64() < last {
		return nil, errors.Errorf("%w: head %d is below the previous inclusion block %d", ErrStaleHead, head.Number, last)
	}
	balance, err := m.pinnedBalance(ctx, rpc.BlockNumberOrHashWithHash(head.Hash(), true))
	if err != nil {
		return nil, err
	}
	return affordableMaxFee(balance, value, gas), nil
}

// noteInclusion raises the block every later balance pin must reach to a lifecycle's inclusion block.
func (m *Manager) noteInclusion(block uint64) {
	for {
		current := m.lastInclusion.Load()
		if block <= current || m.lastInclusion.CompareAndSwap(current, block) {
			return
		}
	}
}

func headerNumber(head *types.Header) uint64 {
	if head == nil || head.Number == nil || !head.Number.IsUint64() {
		return 0
	}
	return head.Number.Uint64()
}

// sleepContext waits for d or until ctx ends, returning ctx's error in the latter case.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
