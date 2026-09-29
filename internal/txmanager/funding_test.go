package txmanager

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fundingBackend is a guardBackend that also reads the latest signer balance, so account polls run and
// the funding gate can evaluate.
type fundingBackend struct {
	*guardBackend

	balanceAtCalls int
}

func newFundingBackend(balance *big.Int) *fundingBackend {
	return &fundingBackend{guardBackend: newGuardBackend(balance)}
}

func (b *fundingBackend) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	b.balanceAtCalls++
	if b.balanceErr != nil {
		return nil, b.balanceErr
	}
	return new(big.Int).Set(b.balance), nil
}

func (b *fundingBackend) setNextBase(nextBase *big.Int) {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	b.nextBase = nextBase
}

func TestNextFundable(t *testing.T) {
	need := big.NewInt(1_000)
	for _, tc := range []struct {
		name    string
		state   fundingState
		balance int64
		bps     uint64
		want    bool
	}{
		{name: "first evaluation at the threshold opens", state: fundingUnknown, balance: 1_000, bps: 2000, want: true},
		{name: "first evaluation below the threshold closes", state: fundingUnknown, balance: 999, bps: 2000, want: false},
		{name: "open lane stays open at the threshold", state: fundingOpen, balance: 1_000, bps: 2000, want: true},
		{name: "open lane closes one wei below", state: fundingOpen, balance: 999, bps: 2000, want: false},
		{name: "closed lane stays closed at the threshold", state: fundingClosed, balance: 1_000, bps: 2000, want: false},
		{name: "closed lane stays closed below the hysteresis", state: fundingClosed, balance: 1_199, bps: 2000, want: false},
		{name: "closed lane reopens at the hysteresis", state: fundingClosed, balance: 1_200, bps: 2000, want: true},
		{name: "zero hysteresis reopens at the threshold", state: fundingClosed, balance: 1_000, bps: 0, want: true},
		{name: "full hysteresis needs twice the threshold", state: fundingClosed, balance: 1_999, bps: 10_000, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextFundable(tc.state, big.NewInt(tc.balance), need, tc.bps); got != tc.want {
				t.Fatalf("nextFundable = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFundingThresholdIsTheQuoteHorizon pins the gate at referenceGasUnits × fee(pricingHorizonBlocks,
// floorTip): four blocks of maximum base-fee growth past the next block at the default horizon of 5.
func TestFundingThresholdIsTheQuoteHorizon(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cfg      Config
		nextBase int64
		want     string
	}{
		{
			// 8 gwei grows 9, 10.125, 11.390625, 12.814453125 gwei; plus the 0.02 gwei floor tip.
			name: "RFQ reference fill at the default horizon", nextBase: 8e9,
			cfg:  Config{Balance: BalanceConfig{ReferenceGasUnits: 4_350_000}},
			want: "55829871093750000", // 4.35M × 12.834453125 gwei
		},
		{
			name: "pricing horizon 3 is two blocks of growth", nextBase: 8e9,
			cfg:  Config{Balance: BalanceConfig{ReferenceGasUnits: 1_000_000}, Fees: FeeConfig{PricingHorizonBlocks: 3}},
			want: "10145000000000000", // 1M × (10.125 + 0.02) gwei
		},
		{
			name: "a larger mandatory tipGwei is the floor tip", nextBase: 8e9,
			cfg:  Config{TipGwei: 1, Balance: BalanceConfig{ReferenceGasUnits: 1_000_000}},
			want: "13814453125000000", // 1M × (12.814453125 + 1) gwei
		},
		{
			// A 1-wei base fee still grows by one wei per block.
			name: "testnet one-wei base fee", nextBase: 1,
			cfg:  Config{Balance: BalanceConfig{ReferenceGasUnits: 1_000_000}},
			want: "20000005000000", // 1M × (5 wei + 0.02 gwei)
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(newMockBackend(), mustSigner(t), big.NewInt(1), tc.cfg, logr.Discard())
			if got := m.fundingThreshold(big.NewInt(tc.nextBase)); got.String() != tc.want {
				t.Fatalf("threshold = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFundableIsTrueWhileTheGateIsOff(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend Backend
		cfg     Config
	}{
		{name: "no reference gas", backend: newFundingBackend(big.NewInt(0))},
		{
			name: "backend cannot read the balance", backend: newGuardBackend(big.NewInt(0)),
			cfg: Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.backend, mustSigner(t), big.NewInt(1), tc.cfg, logr.Discard())
			if !m.Fundable() {
				t.Fatal("Fundable = false with the funding gate off")
			}
			m.evaluateFunding(t.Context(), big.NewInt(0), big.NewInt(1e9), 1)
			if !m.Fundable() {
				t.Fatal("an evaluation closed a gate that is off")
			}
		})
	}
}

func TestFundableIsFalseUntilTheFirstEvaluation(t *testing.T) {
	m := New(newFundingBackend(eth(1e18)), mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}}, logr.Discard())
	if m.Fundable() {
		t.Fatal("Fundable = true before the gate evaluated")
	}
	m.evaluateFunding(t.Context(), eth(1e18), big.NewInt(1e9), 10)
	if !m.Fundable() {
		t.Fatal("Fundable = false with a funded balance")
	}
}

func TestFundingGateIgnoresOlderHeads(t *testing.T) {
	m := New(newFundingBackend(eth(1e18)), mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 1_000_000}}, logr.Discard())
	nextBase := big.NewInt(1e9)
	need := m.fundingThreshold(nextBase)
	m.evaluateFunding(t.Context(), need, nextBase, 100)
	m.evaluateFunding(t.Context(), big.NewInt(0), nextBase, 99)
	if !m.Fundable() {
		t.Fatal("an evaluation of an older head closed the gate")
	}
	m.evaluateFunding(t.Context(), big.NewInt(0), nextBase, 100)
	if m.Fundable() {
		t.Fatal("an evaluation of the same head did not close the gate")
	}
}

// TestFundingGateFollowsAccountPolls drives the gate from account polls through a close, a balance
// between the threshold and its hysteresis, and a recovery; every change is announced to lane-state
// subscribers and exported, and only changes are logged.
func TestFundingGateFollowsAccountPolls(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	b := newFundingBackend(eth(1e18))
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	if err != nil {
		t.Fatal(err)
	}
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{
		MaxFeeGwei:          50,
		PollInterval:        time.Millisecond,
		AccountPollInterval: 5 * time.Millisecond,
		Balance:             BalanceConfig{ReferenceGasUnits: 4_350_000, TargetEth: 0.1},
	}, metrics, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	startManagerForTest(t, m)

	waitForFundable(t, m, true)
	need := m.fundingThreshold(big.NewInt(20e9))
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_fundable")); got != 1 {
		t.Fatalf("account_fundable = %v, want 1", got)
	}
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_balance_target_wei")); got != 1e17 {
		t.Fatalf("account_balance_target_wei = %v, want 1e17", got)
	}
	drain(changes)

	b.setBalance(new(big.Int).Sub(need, big.NewInt(1)))
	waitForFundable(t, m, false)
	waitForSignal(t, changes)
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_fundable")); got != 0 {
		t.Fatalf("account_fundable = %v, want 0", got)
	}

	// Back above the threshold but short of the 20% hysteresis: the gate stays closed.
	b.setBalance(withBasisPoints(need, 1999))
	pollsBefore := balancePolls(b)
	for balancePolls(b) < pollsBefore+3 {
		time.Sleep(time.Millisecond)
	}
	if m.Fundable() {
		t.Fatal("gate reopened below the hysteresis")
	}

	b.setBalance(withBasisPoints(need, 2000))
	waitForFundable(t, m, true)
	waitForSignal(t, changes)

	mu.Lock()
	defer mu.Unlock()
	for _, msg := range []string{"lane fundable", "lane fundable again"} {
		if errorLevel, info := countLogs(*logs, msg); errorLevel != 0 || info != 1 {
			t.Fatalf("%q logged %d/%d times at error/info, want 0/1", msg, errorLevel, info)
		}
	}
	closed := 0
	for _, entry := range *logs {
		if strings.Contains(entry, `"msg":"lane unfundable`) {
			if !strings.Contains(entry, `"level":0`) {
				t.Fatalf("unfundable transition not logged at info: %s", entry)
			}
			closed++
		}
	}
	if closed != 1 {
		t.Fatalf("unfundable transition logged %d times, want 1", closed)
	}
}

// TestFundingGateFollowsGuardedSends closes the gate from the fee snapshot of a guarded send, without
// waiting for the next account poll: the send the guard refuses already reports the lane unfundable.
func TestFundingGateFollowsGuardedSends(t *testing.T) {
	b := newFundingBackend(eth(1e18))
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{
		MaxFeeGwei:          50,
		PollInterval:        time.Millisecond,
		AccountPollInterval: time.Hour,
		Balance:             BalanceConfig{ReferenceGasUnits: 4_400_000},
	}, newTestMetrics(t), logr.Discard())
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()
	startManagerForTest(t, m)
	waitForFundable(t, m, true)
	drain(changes)

	b.setBalance(eth(1_000_000))
	res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 4_400_000, Label: "fill"})
	if !errors.Is(res.Err, ErrUnaffordable) || !res.NotAdmitted {
		t.Fatalf("send = %+v, want a NotAdmitted ErrUnaffordable", res)
	}
	if m.Fundable() {
		t.Fatal("gate still open after the guard read an unfundable balance")
	}
	waitForSignal(t, changes)
}

func TestFundingGateKeepsItsStateWhenAPollFails(t *testing.T) {
	b := newFundingBackend(eth(1e18))
	m := New(b, mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}}, logr.Discard())
	if _, err := m.readAccount(t.Context()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if !m.Fundable() {
		t.Fatal("funded lane not fundable")
	}
	b.guardMu.Lock()
	b.balanceErr = errors.New("read endpoint down")
	b.guardMu.Unlock()
	if _, err := m.readAccount(t.Context()); err == nil {
		t.Fatal("poll with a failed balance read succeeded")
	}
	if !m.Fundable() {
		t.Fatal("a failed poll closed the gate")
	}
}

func TestFundingGatePollRefreshesFeeGauges(t *testing.T) {
	b := newFundingBackend(eth(1e18))
	b.setNextBase(big.NewInt(8e9))
	metrics := newTestMetrics(t)
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_350_000}}, metrics, logr.Discard())
	if telemetryErr, fundingErr := m.readAccount(t.Context()); telemetryErr != nil || fundingErr != nil {
		t.Fatalf("poll: telemetry %v, funding %v", telemetryErr, fundingErr)
	}
	assertMetric(t, metrics.nextBaseFee.WithLabelValues(), 8e9)
	assertMetric(t, metrics.requiredBalance.WithLabelValues(requiredBalanceQuote), 55829871093750000)
}

func TestEthToWei(t *testing.T) {
	for _, tc := range []struct {
		eth  float64
		want string
	}{
		{eth: 0.1, want: "100000000000000000"},
		{eth: 0.036, want: "36000000000000000"},
		{eth: 1, want: "1000000000000000000"},
		{eth: 0, want: "0"},
		{eth: 1e-18, want: "1"},
	} {
		if got := ethToWei(tc.eth); got.String() != tc.want {
			t.Fatalf("ethToWei(%v) = %s, want %s", tc.eth, got, tc.want)
		}
	}
}

func TestNotAdmittedReason(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{err: errors.Join(errors.New("send"), ErrUnaffordable), want: "unaffordable"},
		{err: errUnaffordableOneBlock, want: "unaffordable_one_block"},
		{err: ErrStaleHead, want: "stale_head"},
		{err: errNonceLanePaused, want: "nonce_conflict"},
		{err: errManagerStopped, want: "manager_stopped"},
		{err: context.DeadlineExceeded, want: "deadline_exceeded"},
		{err: errors.New("anything else"), want: "other"},
	} {
		if got := NotAdmittedReason(tc.err); got != tc.want {
			t.Fatalf("NotAdmittedReason(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func waitForFundable(t *testing.T, m *Manager, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for m.Fundable() != want {
		if time.Now().After(deadline) {
			t.Fatalf("Fundable did not become %v", want)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForSignal(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
	case <-time.After(2 * time.Second):
		t.Fatal("no lane-state notification")
	}
}

func drain(changes <-chan struct{}) {
	for {
		select {
		case <-changes:
		default:
			return
		}
	}
}

func balancePolls(b *fundingBackend) int {
	b.guardMu.Lock()
	defer b.guardMu.Unlock()
	return b.balanceAtCalls
}

// gaugeFamily returns a collector that yields the single series of one registered metric family.
func gaugeFamily(t *testing.T, reg *prometheus.Registry, name string) prometheus.Collector {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name || len(family.GetMetric()) != 1 {
			continue
		}
		value := family.GetMetric()[0].GetGauge().GetValue()
		return prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "copy"}, func() float64 { return value })
	}
	t.Fatalf("metric family %s not found", name)
	return nil
}

// TestCompetitorFillCancelsBeforeCancelAt is the manager side of the solvers' Obsolete wiring: a fill
// another filler completed while ours was pending is cancelled at its nonce as soon as a receipt sweep
// finds none of ours, long before its CancelAt, with the cancellation reason obsolete. An Obsolete read
// error before that keeps the fill pending.
func TestCompetitorFillCancelsBeforeCancelAt(t *testing.T) {
	logs, log := newLogCapture(0)
	var mu sync.Mutex
	sgnr := mustSigner(t)
	b := &replacementBackend{mockBackend: newMockBackend(), cancellationTo: sgnr.Address()}
	m := New(b, sgnr, big.NewInt(11155111), Config{
		MaxFeeGwei:          50,
		PollInterval:        time.Millisecond,
		ReplacementInterval: time.Hour,
		PendingTimeout:      time.Hour,
	}, logr.New(&lockedSink{sink: log.GetSink(), mu: &mu}))
	startManagerForTest(t, m)

	var status sync.Map // "status" -> string
	status.Store("status", "open")
	readFailed := make(chan struct{})
	var failOnce sync.Once
	cancelAt := time.Now().Add(time.Hour)
	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{0x01}, GasLimit: 21_000, Label: "fill",
		CancelAt: cancelAt,
		Obsolete: func(context.Context) (bool, error) {
			current, _ := status.Load("status")
			switch current {
			case "unavailable":
				failOnce.Do(func() { close(readFailed) })
				return false, errors.New("order API unavailable")
			case "filled":
				return true, nil
			default:
				return false, nil
			}
		},
	})
	if !accepted {
		t.Fatal("fill was not accepted")
	}
	waitForSentTransactions(t, b.mockBackend, 1)

	status.Store("status", "unavailable")
	select {
	case <-readFailed:
	case <-time.After(2 * time.Second):
		t.Fatal("no Obsolete read while pending")
	}
	if cancellation := b.cancellationTransaction(); cancellation != nil {
		t.Fatalf("an Obsolete read error cancelled the fill: %s", cancellation.Hash())
	}

	status.Store("status", "filled")
	select {
	case got := <-result:
		if got.Outcome != OutcomeCancelled {
			t.Fatalf("result = %+v, want the fill cancelled", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("competitor fill did not cancel the pending fill")
	}
	if time.Now().After(cancelAt) {
		t.Fatal("cancellation waited for CancelAt")
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, entry := range *logs {
		if strings.Contains(entry, `"msg":"pending transaction cancellation requested"`) &&
			strings.Contains(entry, `"reason":"obsolete"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("cancellation was not requested with reason obsolete: %s", strings.Join(*logs, "\n"))
	}
}
