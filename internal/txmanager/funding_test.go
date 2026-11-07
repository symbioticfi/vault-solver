package txmanager

import (
	"context"
	"math"
	"math/big"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

type fundingBackend struct {
	*mockBackend

	balance *big.Int
	err     error
	wait    bool
	reads   int
}

func (b *fundingBackend) TransactionSenderBalanceAt(ctx context.Context, _ common.Address, block *big.Int) (*big.Int, error) {
	b.reads++
	if block != nil {
		return nil, errors.New("funding requires fresh balance")
	}
	if b.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return b.balance, b.err
}

func TestFundingCheckPrecedesSigningAndCapture(t *testing.T) {
	readErr := errors.New("balance RPC unavailable")
	for _, tc := range []struct {
		name     string
		balance  *big.Int
		readErr  error
		canceled bool
		wantErr  bool
	}{
		{name: "exact maximum cost", balance: big.NewInt(42_017)},
		{name: "one wei short including value", balance: big.NewInt(42_016), wantErr: true},
		{name: "zero balance", balance: new(big.Int), wantErr: true},
		{name: "missing balance", wantErr: true},
		{name: "negative balance", balance: big.NewInt(-1), wantErr: true},
		{name: "oversized balance", balance: new(big.Int).Lsh(big.NewInt(1), 256), wantErr: true},
		{name: "RPC failure", readErr: readErr, wantErr: true},
		{name: "canceled balance read", balance: big.NewInt(42_017), canceled: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fundingBackend{mockBackend: newMockBackend(), balance: tc.balance, err: tc.readErr}
			signing := &estimateCountingSigner{Signer: mustSigner(t)}
			m := New(backend, signing, big.NewInt(1), Config{}, logr.Discard())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			captures := 0
			signed, err := m.signAndSend(ctx, 7, common.Address{1}, nil, big.NewInt(17), 21_000,
				feeQuote{baseFee: big.NewInt(1), tip: big.NewInt(1), maxFee: big.NewInt(2)}, false,
				func(_, _ *types.Transaction) error { captures++; return nil })
			if tc.wantErr {
				if err == nil || signed != nil || signing.calls.Load() != 0 || captures != 0 || len(backend.attemptedTransactions()) != 0 {
					t.Fatalf("funding failure signed or sent: signed=%v signs=%d captures=%d err=%v", signed != nil, signing.calls.Load(), captures, err)
				}
				if tc.readErr != nil && !errors.Is(err, tc.readErr) {
					t.Fatalf("balance RPC error lost: %v", err)
				}
				if tc.canceled && !errors.Is(err, context.Canceled) {
					t.Fatalf("balance cancellation lost: %v", err)
				}
			} else if err != nil || signed == nil || signing.calls.Load() != 1 || captures != 1 || len(backend.attemptedTransactions()) != 1 {
				t.Fatalf("exact funding rejected: signed=%v signs=%d captures=%d err=%v", signed != nil, signing.calls.Load(), captures, err)
			}
			if backend.reads != 1 {
				t.Fatalf("fresh funding reads = %d", backend.reads)
			}
		})
	}
}

func TestFundingRequiresBalanceCapability(t *testing.T) {
	backend := newMockBackend()
	signing := &estimateCountingSigner{Signer: mustSigner(t)}
	// Expose only the mandatory backend surface, without either balance capability.
	m := New(struct{ Backend }{backend}, signing, big.NewInt(1), Config{}, logr.Discard())
	signed, err := m.signAndSend(t.Context(), 7, common.Address{1}, nil, new(big.Int), 21_000,
		feeQuote{baseFee: big.NewInt(1), tip: big.NewInt(1), maxFee: big.NewInt(2)}, false, nil)
	if err == nil || signed != nil || signing.calls.Load() != 0 || len(backend.attemptedTransactions()) != 0 {
		t.Fatalf("missing balance capability signed or sent: signed=%v signs=%d err=%v", signed != nil, signing.calls.Load(), err)
	}
}

func TestFundingCheckRejectsInvalidMaximumCost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		gas        uint64
		fee, value *big.Int
	}{
		{name: "negative value", gas: 21_000, fee: big.NewInt(2), value: big.NewInt(-1)},
		{name: "negative fee", gas: 21_000, fee: big.NewInt(-1), value: new(big.Int)},
		{name: "overflowing gas cost", gas: math.MaxUint64, fee: new(big.Int).Lsh(big.NewInt(1), 255), value: new(big.Int)},
		{name: "overflowing value and gas cost", gas: 1, fee: big.NewInt(1), value: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fundingBackend{mockBackend: newMockBackend(), balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))}
			signing := &estimateCountingSigner{Signer: mustSigner(t)}
			m := New(backend, signing, big.NewInt(1), Config{}, logr.Discard())
			captures := 0
			signed, err := m.signAndSend(t.Context(), 7, common.Address{1}, nil, tc.value, tc.gas,
				feeQuote{baseFee: new(big.Int), tip: new(big.Int), maxFee: tc.fee}, false,
				func(_, _ *types.Transaction) error { captures++; return nil })
			if err == nil || signed != nil || signing.calls.Load() != 0 || captures != 0 || len(backend.attemptedTransactions()) != 0 {
				t.Fatalf("invalid maximum cost signed or sent: signed=%v signs=%d captures=%d err=%v", signed != nil, signing.calls.Load(), captures, err)
			}
		})
	}
}

func TestFundingCheckUsesFinalReusableFeeFloor(t *testing.T) {
	backend := &fundingBackend{mockBackend: newMockBackend(), balance: big.NewInt(2_500_000_000_000_000)}
	signing := &estimateCountingSigner{Signer: mustSigner(t)}
	m := New(backend, signing, big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	m.rememberReusable(7, feeQuote{baseFee: gweiToWei(20), tip: gweiToWei(4), maxFee: gweiToWei(60)})
	// The 50,000-gas call needs 3.375e15 wei at the bumped 67.5-gwei floor.
	// Its ordinary quote is affordable with this balance.
	pending, err := m.broadcast(t.Context(), Request{To: common.Address{1}, GasLimit: 50_000})
	if err == nil || pending != nil || signing.calls.Load() != 0 || len(backend.attemptedTransactions()) != 0 {
		t.Fatalf("fee floor bypassed funding admission: pending=%v signs=%d err=%v", pending != nil, signing.calls.Load(), err)
	}
}

func TestReplacementFundingFailureRetainsPreviousAttempt(t *testing.T) {
	backend := &fundingBackend{mockBackend: newMockBackend(), balance: new(big.Int).Lsh(big.NewInt(1), 128)}
	signing := &estimateCountingSigner{Signer: mustSigner(t)}
	m := New(backend, signing, big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
	if err := m.Hold("funding-test"); err != nil {
		t.Fatal(err)
	}
	captures := 0
	pending, err := m.broadcast(t.Context(), Request{Owner: "funding-test", To: common.Address{1}, GasLimit: 21_000,
		BeforeBroadcast: func(_, _ *types.Transaction) error { captures++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	original, originalFee := pending.attempts[0].tx, new(big.Int).Set(pending.fees.maxFee)
	delete(backend.receipts, original.Hash()) // Keep the original pending while its replacement is priced.
	backend.balance = new(big.Int)
	if _, err := m.tryReplace(t.Context(), pending, replaceIntent{reason: replaceReasonFallback}); err == nil {
		t.Fatal("unfunded replacement admitted")
	}
	if len(pending.attempts) != 1 || pending.attempts[0].tx.Hash() != original.Hash() || pending.fees.maxFee.Cmp(originalFee) != 0 || signing.calls.Load() != 1 || captures != 1 || len(backend.attemptedTransactions()) != 1 || m.Available() {
		t.Fatal("unfunded replacement lost prior owned attempt or signer reservation")
	}
}

func TestFundingReadHasBoundedContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := &fundingBackend{mockBackend: newMockBackend(), wait: true}
		signing := &estimateCountingSigner{Signer: mustSigner(t)}
		m := New(backend, signing, big.NewInt(1), Config{}, logr.Discard())
		started := time.Now()
		_, err := m.signAndSend(t.Context(), 7, common.Address{1}, nil, new(big.Int), 21_000,
			feeQuote{baseFee: big.NewInt(1), tip: big.NewInt(1), maxFee: big.NewInt(2)}, false, nil)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 5*time.Second || signing.calls.Load() != 0 || !strings.Contains(err.Error(), "fund") {
			t.Fatalf("balance read exceeded unsigned admission budget: elapsed=%v signs=%d err=%v", time.Since(started), signing.calls.Load(), err)
		}
	})
}
