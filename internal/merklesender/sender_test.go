package merklesender

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type fakeHost struct {
	owner    string
	req      txmanager.Request
	results  chan txmanager.Result
	attempts []*types.Transaction
	receipt  *types.Receipt
	preSign  error
	resumes  int
}

func (h *fakeHost) Hold(owner string) error {
	if h.owner != "" && h.owner != owner {
		return txmanager.ErrLaneReserved
	}
	h.owner = owner
	return nil
}
func (h *fakeHost) Release(owner string) error {
	if h.owner != owner {
		return txmanager.ErrLaneReserved
	}
	h.owner = ""
	return nil
}
func (h *fakeHost) SendAsync(_ context.Context, req txmanager.Request) (<-chan txmanager.Result, bool) {
	h.req = req
	h.results = make(chan txmanager.Result, 1)
	if h.preSign != nil {
		h.results <- txmanager.Result{Err: h.preSign, Outcome: txmanager.OutcomeSubmissionError}
		return h.results, true
	}
	unsigned, signed := signedFixture(req, 7, 1)
	if err := req.BeforeBroadcast(unsigned, signed); err != nil {
		h.results <- txmanager.Result{Err: err, Outcome: txmanager.OutcomeSubmissionError}
	} else {
		h.attempts = append(h.attempts, signed)
	}
	return h.results, true
}
func (h *fakeHost) ResumeOwned(_ context.Context, req txmanager.Request, attempts []*types.Transaction) (<-chan txmanager.Result, bool) {
	h.resumes++
	h.req = req
	h.attempts = attempts
	h.results = make(chan txmanager.Result, 1)
	return h.results, true
}
func (h *fakeHost) ConfirmOwnedReceipt(_ context.Context, tx *types.Transaction) (*types.Receipt, error) {
	if h.receipt != nil && h.receipt.TxHash == tx.Hash() {
		return h.receipt, nil
	}
	return nil, nil
}

func signedFixture(req txmanager.Request, nonce uint64, fee int64) (unsignedTx, signedTx *types.Transaction) {
	key, err := crypto.HexToECDSA("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		panic(err)
	}
	unsigned := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: nonce, To: &req.To, Data: req.Data, Gas: 21000, GasFeeCap: big.NewInt(fee), GasTipCap: big.NewInt(1)})
	signed, err := types.SignTx(unsigned, types.LatestSignerForChainID(big.NewInt(1)), key)
	if err != nil {
		panic(err)
	}
	return unsigned, signed
}
func newFakeSender() (*Sender, *fakeHost) {
	h := new(fakeHost)
	return &Sender{manager: h, owner: "test-owner", solver: "test", wakeup: make(chan struct{}, 1)}, h
}
func ownedReceipt(tx *types.Transaction) *types.Receipt {
	return &types.Receipt{TxHash: tx.Hash(), BlockNumber: big.NewInt(9), BlockHash: common.HexToHash("0x1234"), Status: 1}
}

func TestSenderKeepsSignedOwnershipAcrossAmbiguousHostResults(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeAbandoned, txmanager.OutcomeNonceConsumed, txmanager.OutcomeNonceConflict, txmanager.OutcomeTrackingStopped, txmanager.OutcomeSubmissionError, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			s, h := newFakeSender()
			req := transaction.Request{To: common.HexToAddress("0x1234"), Data: []byte{1}}
			if err := s.Submit(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			req.Data[0] = 2
			if h.req.Data[0] != 1 {
				t.Fatal("caller mutated owned calldata")
			}
			original := h.attempts[0]
			h.results <- txmanager.Result{Hash: original.Hash(), Outcome: outcome, Err: errors.New("uncertain")}
			result, err := s.Poll(t.Context())
			if result != nil || err == nil || !s.Busy() || h.owner == "" || h.resumes != 1 || h.attempts[0].Hash() != original.Hash() {
				t.Fatalf("lost ownership: result=%+v err=%v", result, err)
			}
			if err := s.Submit(t.Context(), req); !errors.Is(err, transaction.ErrBusy) {
				t.Fatal("accepted second request", err)
			}
			// A replacement captured by the host remains owned even when its first
			// receipt arrives after the preceding host result was returned.
			unsigned, replacement := signedFixture(h.req, 7, 2)
			if err := h.req.BeforeBroadcast(unsigned, replacement); err != nil {
				t.Fatal(err)
			}
			h.receipt = ownedReceipt(replacement)
			h.results <- txmanager.Result{Hash: replacement.Hash(), Outcome: txmanager.OutcomeConfirmed}
			result, err = s.Poll(t.Context())
			if err != nil || result == nil || result.Hash != replacement.Hash() || s.Busy() || h.owner != "" {
				t.Fatalf("confirmation = %+v %v", result, err)
			}
			if result, err = s.Poll(t.Context()); result != nil || err != nil {
				t.Fatal("terminal result delivered twice")
			}
		})
	}
}

func TestSenderWakeupOnlyForCanonicalCompletion(t *testing.T) {
	for _, outcome := range []txmanager.Outcome{txmanager.OutcomeConfirmed, txmanager.OutcomeReverted, txmanager.OutcomeAbandoned, txmanager.OutcomeNonceConsumed, txmanager.OutcomeNonceConflict, txmanager.OutcomeTrackingStopped, txmanager.OutcomeSubmissionError, txmanager.OutcomeIncludedUnconfirmed} {
		t.Run(string(outcome), func(t *testing.T) {
			s, h := newFakeSender()
			if err := s.Submit(t.Context(), transaction.Request{To: common.HexToAddress("0x1234")}); err != nil {
				t.Fatal(err)
			}
			result := txmanager.Result{Hash: h.attempts[0].Hash(), Outcome: outcome}
			h.results <- result
			// Several completed requests before a host observes the signal coalesce.
			h.req.AfterResult(result)
			h.req.AfterResult(result)
			canonical := outcome == txmanager.OutcomeConfirmed || outcome == txmanager.OutcomeReverted
			select {
			case <-s.Wakeup():
				if !canonical {
					t.Fatal("ambiguous result woke host")
				}
			default:
				if canonical {
					t.Fatal("canonical result did not wake host")
				}
			}
			select {
			case <-s.Wakeup():
				t.Fatal("completion notifications did not coalesce")
			default:
			}
		})
	}
}

func TestSenderPreSignFailuresReleaseReservation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sender func() (*Sender, *fakeHost)
		req    transaction.Request
	}{
		{name: "unsigned host failure", sender: func() (*Sender, *fakeHost) {
			s, h := newFakeSender()
			h.preSign = errors.New("estimate failed")
			return s, h
		}, req: transaction.Request{To: common.HexToAddress("0x1234")}},
		{name: "missing destination", sender: newFakeSender},
		{name: "missing manager", sender: func() (*Sender, *fakeHost) { return New(nil, "test"), new(fakeHost) }, req: transaction.Request{To: common.HexToAddress("0x1234")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, h := tc.sender()
			if err := s.Submit(t.Context(), tc.req); err == nil || s.Busy() || h.owner != "" {
				t.Fatal("pre-sign failure retained a phantom transaction", err)
			}
		})
	}
}

func TestSenderRejectsInvalidReceiptEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.Receipt)
	}{
		{name: "foreign log", mutate: func(r *types.Receipt) { r.Logs = []*types.Log{{TxHash: common.HexToHash("0x55")}} }},
		{name: "removed log", mutate: func(r *types.Receipt) {
			r.Logs = []*types.Log{{TxHash: r.TxHash, BlockHash: r.BlockHash, BlockNumber: 9, Removed: true}}
		}},
		{name: "nil log", mutate: func(r *types.Receipt) { r.Logs = []*types.Log{nil} }},
		{name: "invalid status", mutate: func(r *types.Receipt) { r.Status = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, h := newFakeSender()
			if err := s.Submit(t.Context(), transaction.Request{To: common.HexToAddress("0x1234")}); err != nil {
				t.Fatal(err)
			}
			h.receipt = ownedReceipt(h.attempts[0])
			tc.mutate(h.receipt)
			h.results <- txmanager.Result{Hash: h.attempts[0].Hash(), Outcome: txmanager.OutcomeConfirmed}
			if result, err := s.Poll(t.Context()); result != nil || err == nil || !s.Busy() || h.owner == "" {
				t.Fatalf("invalid receipt resolved ownership: %+v %v", result, err)
			}
			h.receipt = ownedReceipt(h.attempts[0])
			if result, err := s.Poll(t.Context()); result == nil || err != nil {
				t.Fatalf("corrected receipt did not recover: %+v %v", result, err)
			}
		})
	}
}

func TestSenderCapturesCancellationAndRejectsChangedReplacement(t *testing.T) {
	s, h := newFakeSender()
	if err := s.Submit(t.Context(), transaction.Request{To: common.HexToAddress("0x1234")}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		req   txmanager.Request
		nonce uint64
	}{
		{name: "changed nonce", req: h.req, nonce: 8},
		{name: "changed destination", req: txmanager.Request{To: common.HexToAddress("0x5678")}, nonce: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unsigned, signed := signedFixture(tc.req, tc.nonce, 2)
			if err := h.req.BeforeBroadcast(unsigned, signed); err == nil {
				t.Fatal("changed replacement captured")
			}
		})
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Poll(canceled); !errors.Is(err, context.Canceled) || !s.Busy() || h.owner == "" {
		t.Fatal("caller cancellation released signed work", err)
	}
	if len(s.active.snapshot()) != 1 {
		t.Fatal("invalid replacements entered ownership ledger")
	}
}
