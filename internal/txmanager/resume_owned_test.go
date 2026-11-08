package txmanager

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

func TestResumeOwnedKeepsNonceAfterAmbiguousOutcome(t *testing.T) {
	b := &silentAcceptanceBackend{mockBackend: newMockBackend()}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, PendingTimeout: 10 * time.Millisecond, ShutdownTimeout: time.Millisecond}, logr.Discard())
	startManagerForTest(t, m)
	if err := m.Hold("ram"); err != nil {
		t.Fatal(err)
	}
	var attempts []*types.Transaction
	req := Request{Owner: "ram", To: common.HexToAddress("0x1234"), GasLimit: 21000, BeforeBroadcast: func(_, signed *types.Transaction) error { attempts = append(attempts, signed); return nil }}
	first := m.Send(t.Context(), req)
	if first.Outcome != OutcomeAbandoned || len(attempts) != 1 {
		t.Fatalf("initial = %+v attempts=%d", first, len(attempts))
	}
	if m.Available() {
		t.Fatal("ambiguous result released reservation")
	}
	// A lagging receipt appears after the host lifecycle timed out. An expired
	// business deadline permits receipt observation, never another broadcast.
	req.Deadline = time.Now().Add(-time.Second)
	b.mu.Lock()
	b.receipts[attempts[0].Hash()] = successfulReceipt(attempts[0], b.head)
	b.mu.Unlock()
	resumed, accepted := m.ResumeOwned(t.Context(), req, attempts)
	if !accepted {
		t.Fatal("owned lifecycle not admitted")
	}
	select {
	case got := <-resumed:
		if got.Outcome != OutcomeConfirmed || got.Hash != first.Hash {
			t.Fatalf("resume = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("owned receipt not observed")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 1 || b.estimateCalls.Load() != 0 {
		t.Fatal("resume rebuilt or rebroadcast the expired request")
	}
}

func TestConfirmOwnedReceiptRequiresCanonicalDepth(t *testing.T) {
	for _, tc := range []struct {
		name      string
		depth     uint64
		orphaned  bool
		confirmed bool
	}{
		{name: "canonical", confirmed: true}, {name: "not mature", depth: 1}, {name: "orphaned", orphaned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newMockBackend()
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, Confirmations: tc.depth}, logr.Discard())
			pending, err := m.broadcast(t.Context(), Request{To: common.HexToAddress("0x1234"), GasLimit: 21000})
			if err != nil {
				t.Fatal(err)
			}
			b.reorgedHeader = tc.orphaned
			receipt, err := m.ConfirmOwnedReceipt(t.Context(), pending.attempts[0].tx)
			if (receipt != nil) != tc.confirmed {
				t.Fatalf("receipt=%+v err=%v", receipt, err)
			}
		})
	}
}

func TestResumeOwnedRejectsChangedSignedWork(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Request, []*types.Transaction) []*types.Transaction
	}{
		{name: "nil attempt", mutate: func(_ *Request, txs []*types.Transaction) []*types.Transaction { return append(txs, nil) }},
		{name: "destination", mutate: func(req *Request, txs []*types.Transaction) []*types.Transaction {
			req.To = common.HexToAddress("0xdead")
			return txs
		}},
		{name: "calldata", mutate: func(req *Request, txs []*types.Transaction) []*types.Transaction { req.Data = []byte{99}; return txs }},
		{name: "value", mutate: func(req *Request, txs []*types.Transaction) []*types.Transaction {
			req.Value = big.NewInt(1)
			return txs
		}},
		{name: "owner", mutate: func(req *Request, txs []*types.Transaction) []*types.Transaction { req.Owner = ""; return txs }},
		{name: "capture hook", mutate: func(req *Request, txs []*types.Transaction) []*types.Transaction {
			req.BeforeBroadcast = nil
			return txs
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newMockBackend()
			m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
			req := Request{Owner: "ram", To: common.HexToAddress("0x1234"), GasLimit: 21000, BeforeBroadcast: func(_, _ *types.Transaction) error { return nil }}
			pending, err := m.broadcast(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			attempts := tc.mutate(&req, []*types.Transaction{pending.attempts[0].tx})
			result, accepted := m.ResumeOwned(t.Context(), req, attempts)
			if !accepted {
				t.Fatal("validation failure channel absent")
			}
			if got := <-result; got.Err == nil || !got.NotAdmitted {
				t.Fatalf("changed work admitted: %+v", got)
			}
			if b.sendCalls != 1 {
				t.Fatal("changed work broadcast")
			}
		})
	}
}

type ownedBoundarySigner struct {
	signer.Signer

	changePayload bool
	afterSigning  func()
}

func (s ownedBoundarySigner) SignTx(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	if s.changePayload {
		tx = types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: tx.Nonce() + 1, To: tx.To(), Data: tx.Data(), Value: tx.Value(), Gas: tx.Gas(), GasFeeCap: tx.GasFeeCap(), GasTipCap: tx.GasTipCap()})
	}
	signed, err := s.Signer.SignTx(ctx, tx, chainID)
	if s.afterSigning != nil {
		s.afterSigning()
	}
	return signed, err
}

func TestSignedOwnershipCaptureBoundary(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		change, cancel, capture, wantOwned bool
	}{
		{name: "changed signer payload", change: true, capture: true},
		{name: "canceled after signing", cancel: true, capture: true, wantOwned: true},
		{name: "ordinary canceled send", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newMockBackend()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			signing := ownedBoundarySigner{Signer: mustSigner(t), changePayload: tc.change}
			if tc.cancel {
				signing.afterSigning = cancel
			}
			m := New(b, signing, big.NewInt(1), Config{MaxFeeGwei: 100}, logr.Discard())
			captured := false
			var hook func(_, _ *types.Transaction) error
			if tc.capture {
				hook = func(_, _ *types.Transaction) error { captured = true; return nil }
			}
			signed, err := m.signAndSend(ctx, 7, common.HexToAddress("0x1234"), nil, new(big.Int), 21000, feeQuote{baseFee: big.NewInt(1), tip: big.NewInt(1), maxFee: big.NewInt(2)}, false, hook)
			if err == nil || (signed != nil) != tc.wantOwned || captured != tc.wantOwned || b.sendCalls != 0 {
				t.Fatalf("signed=%v captured=%v sends=%d err=%v", signed != nil, captured, b.sendCalls, err)
			}
		})
	}
}

func TestResumeOwnedReplacesCapturedRPCRejectionAtOriginalNonce(t *testing.T) {
	b := newMockBackend()
	b.sendErrs = []error{errors.New("insufficient funds")}
	m := New(b, mustSigner(t), big.NewInt(1), Config{MaxFeeGwei: 100, PollInterval: time.Millisecond, ReplacementInterval: 2 * time.Millisecond, PendingTimeout: time.Second, ShutdownTimeout: time.Millisecond, Horizon: HorizonConfig{BlockTime: 4 * time.Millisecond}}, logr.Discard())
	startManagerForTest(t, m)
	if err := m.Hold("retained-call"); err != nil {
		t.Fatal(err)
	}
	captured := make(chan *types.Transaction, 4)
	req := Request{Owner: "retained-call", To: common.HexToAddress("0x1234"), Data: []byte{1, 2, 3}, Value: big.NewInt(17), GasLimit: 21000, BeforeBroadcast: func(unsigned, signed *types.Transaction) error {
		if types.LatestSignerForChainID(big.NewInt(1)).Hash(unsigned) != types.LatestSignerForChainID(big.NewInt(1)).Hash(signed) {
			return errors.New("capture received changed signed payload")
		}
		captured <- signed
		return nil
	}}
	rejected := m.Send(t.Context(), req)
	if rejected.Outcome != OutcomeSubmissionError || rejected.Err == nil {
		t.Fatalf("initial RPC rejection = %+v", rejected)
	}
	original := <-captured
	if m.Available() {
		t.Fatal("captured RPC rejection released signer reservation")
	}
	// A stale mined-nonce endpoint now reports six instead of the originally
	// selected seven. A fresh send would choose six; retained work must keep seven.
	b.mu.Lock()
	b.latestNonce, b.pendingNonce = 6, 6
	b.mu.Unlock()
	b.mine(gweiToWei(33))
	resumed, accepted := m.ResumeOwned(t.Context(), req, []*types.Transaction{original})
	if !accepted {
		t.Fatal("retained rejection not admitted")
	}
	var result Result
	select {
	case result = <-resumed:
	case <-time.After(time.Second):
		t.Fatal("resumed host did not sign a replacement")
	}
	if result.Outcome != OutcomeConfirmed || result.Err != nil {
		t.Fatalf("replacement result = %+v", result)
	}
	var replacement *types.Transaction
	select {
	case replacement = <-captured:
	default:
		t.Fatal("replacement bypassed ownership capture")
	}
	if replacement.Hash() == original.Hash() || replacement.Hash() != result.Hash || replacement.Nonce() != original.Nonce() || replacement.Nonce() != 7 || replacement.To() == nil || *replacement.To() != req.To || !bytes.Equal(replacement.Data(), req.Data) || replacement.Value().Cmp(req.Value) != 0 {
		t.Fatalf("resumption changed owned identity: nonce=%d original=%d hash=%s", replacement.Nonce(), original.Nonce(), replacement.Hash())
	}
	if replacement.GasFeeCap().Cmp(bumpFee(original.GasFeeCap())) < 0 || replacement.GasTipCap().Cmp(bumpFee(original.GasTipCap())) < 0 {
		t.Fatal("resumed replacement bypassed host fee policy")
	}
	if len(result.Attempts) != 2 || result.Attempts[0] != original.Hash() || result.Attempts[1] != replacement.Hash() {
		t.Fatalf("resumed ownership hashes = %v", result.Attempts)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendCalls != 2 || len(b.sent) != 1 || b.sent[0].Hash() != replacement.Hash() {
		t.Fatal("resumption rebuilt or resent the original request before replacement")
	}
}
