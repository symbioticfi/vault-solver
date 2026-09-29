package txmanager

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

// TestSignerBalanceFollowsAccountPolls covers the funding gate being off (balance.referenceGasUnits 0, as
// for 3F): every account poll records the signer balance it read, so a solver backing off after
// ErrUnaffordable can see the signer funded. No read, rise or drop, is a lane-state signal: subscribers
// such as LI.FI retire and republish standing quotes on every signal.
func TestSignerBalanceFollowsAccountPolls(t *testing.T) {
	b := newFundingBackend(eth(1_000))
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{}, newTestMetrics(t), logr.Discard())
	if m.fundingGateOn {
		t.Fatal("funding gate on without balance.referenceGasUnits")
	}
	if m.SignerBalance() != nil {
		t.Fatal("SignerBalance set before any read")
	}
	changes, unsubscribe := m.SubscribeLaneState()
	defer unsubscribe()

	for _, balance := range []int64{1_000, 900, 900, 901} {
		b.setBalance(eth(balance))
		m.refreshAccount(t.Context())
		if got := m.SignerBalance(); got == nil || got.Cmp(eth(balance)) != 0 {
			t.Fatalf("SignerBalance = %v, want %d", got, balance)
		}
		select {
		case <-changes:
			t.Fatalf("balance read %d signalled a lane-state change", balance)
		default:
		}
	}
	if got := m.SignerBalance(); got == m.SignerBalance() {
		t.Fatal("SignerBalance returned its internal value instead of a copy")
	}
}

// TestRefusedSendRecordsTheSignerBalance: the balance an ErrUnaffordable refusal was priced against is the
// manager's last read, so a solver can compare later reads with it.
func TestRefusedSendRecordsTheSignerBalance(t *testing.T) {
	b := newGuardBackend(eth(1_000_000))
	m := newGuardManager(t, b, Config{}, nil)

	res := m.Send(t.Context(), Request{To: common.HexToAddress("0xabc"), GasLimit: 4_400_000, Label: "redeem"})
	if !errors.Is(res.Err, ErrUnaffordable) || !res.NotAdmitted {
		t.Fatalf("send = %+v, want a NotAdmitted ErrUnaffordable", res)
	}
	if got := m.SignerBalance(); got == nil || got.Cmp(eth(1_000_000)) != 0 {
		t.Fatalf("SignerBalance = %v, want the refused balance 1000000", got)
	}
}

// TestFundingPollPinsTheBalanceToTheFeeHistoryHead: an account poll evaluates the gate from the balance at
// the fee history's newest block, the block its next base fee belongs to, never at latest (strategy §2.4);
// the head the gate records is then the balance's block. A head below the previous lifecycle's inclusion
// block may predate that fill's payment and is not evaluated.
func TestFundingPollPinsTheBalanceToTheFeeHistoryHead(t *testing.T) {
	b := newFundingBackend(eth(1e18))
	m := New(b, mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}}, logr.Discard())
	if _, err := m.readAccount(t.Context()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !m.Fundable() {
		t.Fatal("funded lane not fundable")
	}
	pins, numbers := b.pins()
	if len(pins) != 1 || numbers[0] != 100 {
		t.Fatalf("balance reads = %v at blocks %v, want one read at fee history head 100", pins, numbers)
	}
	if _, byHash := pins[0].Hash(); byHash {
		t.Fatalf("poll pinned the balance by hash %v, want the fee history's block number", pins[0])
	}
	if calls := balancePolls(b); calls != 0 {
		t.Fatalf("poll read the balance at latest %d times, want none", calls)
	}

	m.noteInclusion(101)
	b.setBalance(big.NewInt(0))
	if _, err := m.readAccount(t.Context()); err == nil {
		t.Fatal("poll below the previous inclusion block evaluated the gate")
	}
	if !m.Fundable() {
		t.Fatal("a balance that may predate the previous fill's payment closed the gate")
	}
}

// nonceFailingBackend fails the account telemetry's pending-nonce read and nothing else.
type nonceFailingBackend struct{ *fundingBackend }

func (nonceFailingBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 0, errors.New("write endpoint down")
}

// TestFundingPollIsIndependentOfTelemetry: the telemetry snapshot and the funding gate are refreshed
// apart, so a failed nonce read does not keep the gate from evaluating, and account_refreshes_total counts
// only the telemetry snapshot.
func TestFundingPollIsIndependentOfTelemetry(t *testing.T) {
	metrics := newTestMetrics(t)
	m := NewWithMetrics(nonceFailingBackend{newFundingBackend(eth(1e18))}, mustSigner(t), big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}}, metrics, logr.Discard())
	m.refreshAccount(t.Context())
	if !m.Fundable() {
		t.Fatal("a failed nonce read kept the funding gate from evaluating")
	}
	assertAccountRefreshes(t, metrics.account, 0, 1)
}

// TestFundingPollFailuresAreLoggedOncePerEpisode: a gate that cannot refresh (here it never evaluates, so
// every quote gated on it declines) is logged at Info when the failures start and when they end, not on
// every poll, and does not count the telemetry snapshot that succeeded as an error. account_fundable is
// exported as 0 while the gate has not evaluated, so the funding alert sees the closed lane.
func TestFundingPollFailuresAreLoggedOncePerEpisode(t *testing.T) {
	logs, log := newLogCapture(0)
	b := newFundingBackend(eth(1e18))
	b.mu.Lock()
	b.historyErr = errors.New("fee history unavailable")
	b.mu.Unlock()
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	if err != nil {
		t.Fatal(err)
	}
	sgnr := mustSigner(t)
	metrics.bindAccount(sgnr.Address())
	m := NewWithMetrics(b, sgnr, big.NewInt(1),
		Config{Balance: BalanceConfig{ReferenceGasUnits: 4_400_000}}, metrics, log)
	m.exportFundingGate()
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_fundable")); got != 0 {
		t.Fatalf("account_fundable before the first evaluation = %v, want 0", got)
	}

	ctx := observability.WithLogger(t.Context(), log)
	for range 3 {
		m.refreshAccount(ctx)
	}
	if m.Fundable() {
		t.Fatal("gate opened without an evaluation")
	}
	assertAccountRefreshes(t, metrics.account, 3, 0)
	if errorLevel, info := countLogs(*logs, "funding gate refresh failed"); errorLevel != 0 || info != 1 {
		t.Fatalf("refresh failures logged %d/%d times at error/info, want 0/1", errorLevel, info)
	}

	b.mu.Lock()
	b.historyErr = nil
	b.mu.Unlock()
	m.refreshAccount(ctx)
	if !m.Fundable() {
		t.Fatal("gate did not evaluate once the reads recovered")
	}
	if errorLevel, info := countLogs(*logs, "funding gate refresh recovered"); errorLevel != 0 || info != 1 {
		t.Fatalf("recovery logged %d/%d times at error/info, want 0/1", errorLevel, info)
	}
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_fundable")); got != 1 {
		t.Fatalf("account_fundable = %v, want 1", got)
	}
}

// TestAccountFundableIsExportedFromStart: with the gate on, Start exports account_fundable as 0 before
// the first poll, instead of leaving the series absent while quoting is gated off.
func TestAccountFundableIsExportedFromStart(t *testing.T) {
	b := newFundingBackend(eth(1e18))
	b.mu.Lock()
	b.historyErr = errors.New("fee history unavailable")
	b.mu.Unlock()
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	if err != nil {
		t.Fatal(err)
	}
	m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{
		PollInterval: time.Millisecond, AccountPollInterval: time.Hour,
		Balance: BalanceConfig{ReferenceGasUnits: 4_400_000},
	}, metrics, logr.Discard())
	startManagerForTest(t, m)
	deadline := time.Now().Add(2 * time.Second)
	for !hasFamily(t, reg, "solver_bot_txmanager_account_fundable") {
		if time.Now().After(deadline) {
			t.Fatal("account_fundable not exported while the gate has not evaluated")
		}
		time.Sleep(time.Millisecond)
	}
	if got := testutil.ToFloat64(gaugeFamily(t, reg, "solver_bot_txmanager_account_fundable")); got != 0 {
		t.Fatalf("account_fundable = %v, want 0", got)
	}
}

func hasFamily(t *testing.T, reg *prometheus.Registry, name string) bool {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}
	return false
}
