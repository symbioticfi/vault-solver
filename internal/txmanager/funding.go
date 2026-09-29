package txmanager

import (
	"context"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/params"
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
// the lane-state notification. Fundable reads only the atomic.

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
// history, and the signer balance the poll already read (or reads now when account metrics are off). It
// also refreshes the fee gauges at poll cadence, sized for balance.referenceGasUnits.
func (m *Manager) refreshFunding(ctx context.Context, balance *big.Int) error {
	head, nextBase, err := m.readNextBaseFee(ctx)
	if err != nil {
		return errors.Errorf("funding gate: %w", err)
	}
	if balance == nil {
		if balance, err = m.accountBalance(ctx); err != nil {
			return errors.Errorf("funding gate: signer balance: %w", err)
		}
		if balance == nil || balance.Sign() < 0 {
			return errors.New("funding gate: invalid signer balance")
		}
	}
	m.observeFeeSnapshot(nextBase, m.cfg.Balance.ReferenceGasUnits)
	m.evaluateFunding(ctx, balance, nextBase, head)
	return nil
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
