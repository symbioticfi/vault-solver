package txmanager

import (
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

func TestAbandonedFeeHintExpiresForFreshPricingAndSending(t *testing.T) {
	for _, operation := range []string{"profitability quote", "fresh send", "cooldown wait"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
				m := New(b, mustSigner(t), big.NewInt(1), Config{
					MaxFeeGwei: 100, PendingTimeout: time.Second, PollInterval: time.Second,
					Horizon: HorizonConfig{BlockTime: 2 * time.Second},
				}, logr.Discard())
				m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
				if operation == "cooldown wait" {
					m.startNonceCooldown(7)
				} else {
					time.Sleep(time.Second) // The hint expires exactly at the pending-timeout boundary.
				}
				if operation == "profitability quote" {
					ceiling, err := m.MaxFeePerGas(t.Context())
					if err != nil || ceiling.Cmp(gweiToWei(60)) >= 0 || m.reusableSnapshot() != nil {
						t.Fatalf("expired hint kept pricing stale fees: ceiling=%v err=%v hint=%+v", ceiling, err, m.reusableSnapshot())
					}
					return
				}
				pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 50_000})
				if err != nil || pending == nil || pending.nonce != 7 || pending.fees.maxFee.Cmp(gweiToWei(60)) >= 0 || m.reusableSnapshot() != nil {
					t.Fatalf("expired hint kept fresh work at stale fees: pending=%+v err=%v hint=%+v", pending, err, m.reusableSnapshot())
				}
			})
		})
	}
}

func TestUnhonorableAbandonedFeeHintFallsBackWithinCeilings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		global     float64
		request    *big.Int
		quoteFirst bool
	}{
		{name: "profitability global ceiling", global: 65, quoteFirst: true},
		{name: "broadcast global ceiling", global: 65},
		{name: "broadcast request ceiling", global: 100, request: gweiToWei(50)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
			metrics := newTestMetrics(t)
			m := NewWithMetrics(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: tc.global}, metrics, logr.Discard())
			m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
			request := Request{Label: "rfq-fill", To: common.Address{1}, GasLimit: 50_000, MaxFeePerGas: tc.request}
			if tc.quoteFirst {
				ceiling, err := m.MaxFeePerGas(t.Context())
				if err != nil || ceiling == nil || m.reusableSnapshot() != nil {
					t.Fatalf("unhonorable hint blocked fresh pricing: ceiling=%v err=%v hint=%+v", ceiling, err, m.reusableSnapshot())
				}
				request.MaxFeePerGas = ceiling
			}
			pending, err := m.broadcast(t.Context(), request)
			if err != nil || pending == nil || pending.nonce != 7 || m.reusableSnapshot() != nil {
				t.Fatalf("unhonorable hint blocked fresh sending: pending=%+v err=%v hint=%+v", pending, err, m.reusableSnapshot())
			}
			if pending.fees.maxFee.Cmp(m.normalFeeLimit(request)) > 0 || pending.fees.tip.Cmp(pending.fees.maxFee) > 0 {
				t.Fatalf("fresh fees exceeded their ceilings: %+v", pending.fees)
			}
			assertMetric(t, metrics.feeLimits.WithLabelValues("rfq-fill", feeLimitPhaseInitial), 0)
		})
	}
}

func TestRejectedFreshFeesNeverCreateOrRatchetAbandonedHint(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "no owned hint"
		if existing {
			name = "existing owned hint"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := newMockBackend()
				b.sendErrs = []error{errors.New("replacement transaction underpriced"), errors.New("replacement transaction underpriced")}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, Horizon: HorizonConfig{BlockTime: time.Millisecond}}, logr.Discard())
				if existing {
					m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
				}
				before := m.reusableSnapshot()
				first, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 50_000})
				if err != nil || first == nil || first.broadcastErr == nil {
					t.Fatalf("first rejection did not return a conflict: pending=%+v err=%v", first, err)
				}
				second, err := m.broadcast(t.Context(), Request{To: common.Address{2}, GasLimit: 50_000})
				if err != nil || second == nil || second.broadcastErr == nil {
					t.Fatalf("second rejection did not return a conflict: pending=%+v err=%v", second, err)
				}
				if second.fees.maxFee.Cmp(first.fees.maxFee) != 0 || second.fees.tip.Cmp(first.fees.tip) != 0 {
					t.Fatalf("rejected fresh fees ratcheted: first=%+v second=%+v", first.fees, second.fees)
				}
				after := m.reusableSnapshot()
				if before == nil {
					if after != nil {
						t.Fatalf("rejected fees created an owned hint: %+v", after)
					}
				} else if after == nil || after.fees.maxFee.Cmp(before.fees.maxFee) != 0 || after.fees.tip.Cmp(before.fees.tip) != 0 {
					t.Fatalf("rejected fees changed the owned hint: before=%+v after=%+v", before, after)
				}
				if existing {
					time.Sleep(m.cfg.PendingTimeout - time.Millisecond)
					if m.reusableSnapshot() != nil {
						t.Fatal("rejected sends renewed the old owned hint's lifetime")
					}
				}
			})
		})
	}
}

func TestExpiredHighFeeHintDoesNotContaminateNewAbandonment(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
		m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PendingTimeout: time.Second}, logr.Discard())
		m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(80)})
		time.Sleep(time.Second)
		m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(1), maxFee: gweiToWei(40)})
		ceiling, err := m.MaxFeePerGas(t.Context())
		if err != nil || ceiling.Cmp(gweiToWei(80)) >= 0 {
			t.Fatalf("expired abandoned fees contaminated a newer hint: ceiling=%v err=%v", ceiling, err)
		}
	})
}

func TestOlderNonceReadCannotClearRenewedSameFeeHint(t *testing.T) {
	for _, operation := range []string{"profitability quote", "nonce selection"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &oldNonceReadBackend{mockBackend: newMockBackend(), entered: make(chan struct{}), release: make(chan struct{})}
				m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
				fees := feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)}
				m.rememberReusable(7, fees)
				done := make(chan error, 1)
				go func() {
					if operation == "profitability quote" {
						_, err := m.MaxFeePerGas(t.Context())
						done <- err
						return
					}
					_, _, err := m.selectNonce(t.Context())
					done <- err
				}()
				<-b.entered
				time.Sleep(time.Millisecond)
				m.rememberReusable(7, fees) // A newer accepted/uncertain lifecycle renewed these exact fees.
				close(b.release)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if m.reusableSnapshot() == nil {
					t.Fatal("stale nonce view erased a newer same-fee hint")
				}
			})
		})
	}
}
