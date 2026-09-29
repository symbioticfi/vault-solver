package txmanager

import (
	"context"
	"math"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// The lane funding gate (strategy §2.10, docs/TXMANAGER-PLAN.md §4.2) tells solvers whether the signer
// balance still funds a reference fill priced at the pricing horizon, so a lane stops making new
// commitments before the balance guard would refuse to send them. It is advisory and separate from
// Available: Available drives process readiness, which must not flap with the base fee. The guard stays
// authoritative for every attempt.
//
// Goroutine model: the account-poll goroutine and the worker goroutine (from a guarded broadcast's fee
// snapshot) both evaluate the gate; fundingGate.mu serializes their evaluations, the transition log and
// the lane-state notification. Fundable reads only the atomic. Manager.fundingReads, the poll's read
// failure streak, belongs to the account-poll goroutine alone.
//
// The manager also keeps the signer balance it read last (SignerBalance), from account polls and from the
// pinned reads of guarded sends and replacements; balanceMu serializes those goroutines' updates. It is
// independent of the gate, so a solver can notice that the signer was funded even while
// balance.referenceGasUnits is 0.

// fundingState is the gate's last evaluation.
type fundingState uint8

const (
	fundingUnknown fundingState = iota
	fundingOpen
	fundingClosed
)

// fundingGate is the state of the lane funding gate.
type fundingGate struct {
	mu    sync.Mutex
	state fundingState
	// head is the fee-history block of the evaluation that set state. An evaluation of an older block
	// (a lagging upstream, or a snapshot read before a newer poll finished) cannot override it.
	head uint64
	open atomic.Bool
}

// Fundable reports whether the signer balance funds a reference fill of balance.referenceGasUnits gas at
// fees.pricingHorizonBlocks blocks of maximum base-fee growth and the floor tip, with
// balance.fundingHysteresisBps of extra balance required to reopen once closed. It is recomputed on every
// account poll and every guarded send, and each change is announced through SubscribeLaneState.
//
// It is always true while the gate is off: balance.referenceGasUnits is 0, or the backend cannot read the
// signer balance. It is false from startup until the first evaluation. Solvers gate new commitments on it
// in addition to LaneReady or Available; it is deliberately not part of Available, which drives process
// readiness and would take quote servers out of rotation on every base-fee spike.
func (m *Manager) Fundable() bool {
	return !m.fundingGateOn || m.funding.open.Load()
}

// fundingThreshold is the balance a reference fill needs at the pricing horizon: referenceGasUnits ×
// fee(pricingHorizonBlocks, floorTip). The guard's refusal floor is fee(minHorizonBlocks + lag), so a lane
// the gate keeps open still has pricingHorizonBlocks − minHorizonBlocks blocks of base-fee growth before a
// quote it made is refused at fill time.
func (m *Manager) fundingThreshold(nextBase *big.Int) *big.Int {
	fee := horizonFee(nextBase, m.cfg.Fees.PricingHorizonBlocks, m.floorTip())
	return requiredBalance(m.cfg.Balance.ReferenceGasUnits, fee, new(big.Int))
}

// nextFundable applies the gate's hysteresis. An open gate stays open while balance ≥ need; a closed one
// reopens only at need plus hysteresisBps basis points, so a balance hovering at the threshold does not
// flap the lane. The first evaluation (unknown state) uses the plain threshold: startup is not a recovery.
func nextFundable(state fundingState, balance, need *big.Int, hysteresisBps uint64) bool {
	if state != fundingClosed {
		return balance.Cmp(need) >= 0
	}
	return balance.Cmp(withBasisPoints(need, hysteresisBps)) >= 0
}

// withBasisPoints is value + floor(value × bps / 10000).
func withBasisPoints(value *big.Int, bps uint64) *big.Int {
	extra := new(big.Int).Mul(value, new(big.Int).SetUint64(bps))
	extra.Quo(extra, big.NewInt(basisPoints))
	return extra.Add(extra, value)
}

// evaluateFunding recomputes the gate from a balance and the next block's base fee observed at head.
// Changes are logged and announced to lane-state subscribers. The close is logged at Info, not Error:
// it is an expected state of an underfunded lane or a base-fee spike, and paging is the alert's job
// (account_fundable with account_balance_target_wei), not the log line's.
func (m *Manager) evaluateFunding(ctx context.Context, balance, nextBase *big.Int, head uint64) {
	if !m.fundingGateOn || balance == nil || nextBase == nil || balance.Sign() < 0 || nextBase.Sign() < 0 {
		return
	}
	need := m.fundingThreshold(nextBase)
	hysteresis := *m.cfg.Balance.FundingHysteresisBps

	m.funding.mu.Lock()
	defer m.funding.mu.Unlock()
	previous := m.funding.state
	if previous != fundingUnknown && head < m.funding.head {
		return
	}
	fundable := nextFundable(previous, balance, need, hysteresis)
	next := fundingClosed
	if fundable {
		next = fundingOpen
	}
	m.funding.state, m.funding.head = next, head
	m.funding.open.Store(fundable)
	m.metrics.observeFundable(fundable)
	if next == previous {
		return
	}
	fields := []any{
		"from", m.signer.Address().Hex(),
		"balance", balance.String(),
		"requiredBalance", need.String(),
		"reopensAt", withBasisPoints(need, hysteresis).String(),
		"nextBaseFee", nextBase.String(),
		"head", head,
		"referenceGasUnits", m.cfg.Balance.ReferenceGasUnits,
		"pricingHorizonBlocks", m.cfg.Fees.PricingHorizonBlocks,
	}
	switch {
	case !fundable:
		observability.Log(ctx).Info("lane unfundable: the signer balance cannot fund a reference fill at the "+
			"pricing horizon; solvers stop new commitments until it is funded", fields...)
	case previous == fundingClosed:
		observability.Log(ctx).Info("lane fundable again", fields...)
	default:
		observability.Log(ctx).Info("lane fundable", fields...)
	}
	m.notifyLaneStateChange()
}

// refreshFunding evaluates the gate on an account poll: the next block's base fee from a one-block fee
// history, and the signer balance at that history's newest block. It also refreshes the fee gauges at poll
// cadence, sized for balance.referenceGasUnits.
func (m *Manager) refreshFunding(ctx context.Context) error {
	head, nextBase, err := m.readNextBaseFee(ctx)
	if err != nil {
		return errors.Errorf("funding gate: %w", err)
	}
	balance, err := m.fundingBalance(ctx, head)
	if err != nil {
		return errors.Errorf("funding gate: %w", err)
	}
	m.observeFeeSnapshot(nextBase, m.cfg.Balance.ReferenceGasUnits)
	m.evaluateFunding(ctx, balance, nextBase, head)
	return nil
}

// fundingBalance reads the signer balance a poll evaluates the gate with, at head, the fee history's
// newest block: the balance and the next base fee then describe the same block, so the gate's head order
// (evaluateFunding) holds for the balance too, and a lagging upstream cannot answer with a balance from
// before the previous fill was paid (strategy §2.4: never at latest). It is read the way the balance guard
// reads it, and a head below the previous lifecycle's inclusion block is refused. With the guard off (the
// backend cannot pin a read, or balance.guard is false) the gate reads latest, as account telemetry does.
func (m *Manager) fundingBalance(ctx context.Context, head uint64) (*big.Int, error) {
	if !m.guardEnabled() {
		balance, err := m.accountBalance(ctx)
		if err != nil {
			return nil, errors.Errorf("signer balance: %w", err)
		}
		if balance == nil || balance.Sign() < 0 {
			return nil, errors.New("invalid signer balance")
		}
		m.observeSignerBalance(balance)
		return balance, nil
	}
	if last := m.lastInclusion.Load(); head < last {
		return nil, errors.Errorf("%w: fee history head %d is below the previous inclusion block %d", ErrStaleHead, head, last)
	}
	if head > math.MaxInt64 {
		return nil, errors.Errorf("%w: fee history head %d is not a block number", ErrStaleHead, head)
	}
	return m.pinnedBalance(ctx, rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(int64(head))))
}

// fundingPollFailed records an account poll that could not refresh the gate. The first failure of a run
// is logged at Info and the run's end at Info again (fundingPollRecovered); the lines between are V(1),
// and a run that outlasts readFailureReminderInterval is raised at Error. The gate keeps its last state
// meanwhile: closed if it never evaluated, so solvers keep declining new commitments.
func (m *Manager) fundingPollFailed(ctx context.Context, err error) {
	// fundable and evaluated say what the gate keeps meaning while it cannot refresh.
	m.fundingReads.failed(observability.Log(ctx), err, "funding gate refresh failed",
		"from", m.signer.Address().Hex(), "fundable", m.Fundable(), "evaluated", m.fundingEvaluated())
}

func (m *Manager) fundingPollRecovered(ctx context.Context) {
	m.fundingReads.recovered(observability.Log(ctx), "funding gate refresh recovered",
		"from", m.signer.Address().Hex(), "fundable", m.Fundable())
}

// fundingEvaluated reports whether the gate has evaluated since startup.
func (m *Manager) fundingEvaluated() bool {
	m.funding.mu.Lock()
	defer m.funding.mu.Unlock()
	return m.funding.state != fundingUnknown
}

// exportFundingGate exports the gate's current state, so account_fundable is 0 from startup until the
// first evaluation instead of absent: Fundable already reports false then, and the funding alert and
// dashboard should see the same.
func (m *Manager) exportFundingGate() {
	if !m.fundingGateOn {
		return
	}
	m.funding.mu.Lock()
	defer m.funding.mu.Unlock()
	m.metrics.observeFundable(m.funding.open.Load())
}

// SignerBalance returns the signer balance the manager read last, from an account poll or the pinned read
// of a guarded send or replacement, or nil before the first read. Right after an ErrUnaffordable result it
// is normally the balance the refusal was priced against, so a solver backing off after ErrUnaffordable can
// compare later reads with it and end the backoff once the signer is funded, whether or not the funding
// gate is on. Account polls refresh it every accountPollIntervalMs while account metrics or the gate are
// on. Reads at latest can come from a lagging upstream, so a rise is a hint to retry, not proof of funds;
// the balance guard still decides every attempt. A rise is not announced through SubscribeLaneState:
// subscribers such as LI.FI retire and republish standing quotes on every signal.
func (m *Manager) SignerBalance() *big.Int {
	m.balanceMu.Lock()
	defer m.balanceMu.Unlock()
	if m.signerBalance == nil {
		return nil
	}
	return new(big.Int).Set(m.signerBalance)
}

// observeSignerBalance records a signer balance read.
func (m *Manager) observeSignerBalance(balance *big.Int) {
	if balance == nil || balance.Sign() < 0 {
		return
	}
	m.balanceMu.Lock()
	defer m.balanceMu.Unlock()
	m.signerBalance = new(big.Int).Set(balance)
}

// readNextBaseFee reads the newest block and the base fee the node reports for the block after it, from
// eth_feeHistory(1, latest, []): one read, exact on any EIP-1559 chain.
func (m *Manager) readNextBaseFee(ctx context.Context) (uint64, *big.Int, error) {
	history, err := m.backend.FeeHistory(ctx, 1, nil, []float64{})
	if err != nil {
		return 0, nil, errors.Errorf("fee history: %w", err)
	}
	head, nextBase, ok := feeHistoryNext(history)
	if !ok {
		return 0, nil, errors.New("fee history has no next base fee")
	}
	return head, nextBase, nil
}

// accountBalance reads the signer balance at latest the way account telemetry does: through the read
// endpoints when the backend offers them, otherwise through the transaction backend.
func (m *Manager) accountBalance(ctx context.Context) (*big.Int, error) {
	if backend, ok := m.backend.(accountTelemetryBackend); ok {
		return backend.ReadBalanceAt(ctx, m.signer.Address())
	}
	return m.transactionSenderBalance(ctx)
}

// ethToWei converts an ether amount to wei exactly as written in decimal: through its shortest decimal
// form, so 0.1 is 10^17 wei rather than the binary float's 100000000000000005.
func ethToWei(eth float64) *big.Int {
	amount, ok := new(big.Rat).SetString(strconv.FormatFloat(eth, 'f', -1, 64))
	if !ok || amount.Sign() <= 0 {
		return new(big.Int)
	}
	amount.Mul(amount, new(big.Rat).SetInt(big.NewInt(params.Ether)))
	return new(big.Int).Quo(amount.Num(), amount.Denom())
}
