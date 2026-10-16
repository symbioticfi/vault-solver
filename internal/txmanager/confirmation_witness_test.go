package txmanager

import (
	"context"
	"io"
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// witnessBackend gives parent reads constant lookup cost so the number of RPCs,
// rather than the fake's header generation cost, measures confirmation work.
type witnessBackend struct {
	*mockBackend

	headers       []*types.Header
	byHash        map[common.Hash]*types.Header
	latest        uint64
	advancing     bool
	parentReads   int
	numberedReads int
	receiptReads  int
	onNumber      func(*big.Int, int, *types.Header) (*types.Header, error)
	onParent      func(int, *types.Header) (*types.Header, error)
	onReceipt     func(int, *types.Receipt) (*types.Receipt, error)
}

func newWitnessBackend(tx *types.Transaction, gap uint64) *witnessBackend {
	b := &witnessBackend{mockBackend: newMockBackend(), byHash: map[common.Hash]*types.Header{}, latest: 100 + gap}
	b.receipts[tx.Hash()] = successfulReceipt(tx, 100)
	return b
}

func (b *witnessBackend) header(number uint64) *types.Header {
	for uint64(len(b.headers)) <= number {
		n := uint64(len(b.headers))
		h := &types.Header{Number: new(big.Int).SetUint64(n), GasLimit: 60_000_000, BaseFee: big.NewInt(20e9), Time: n * 12}
		if n > 0 {
			h.ParentHash = b.headers[n-1].Hash()
		}
		b.headers = append(b.headers, h)
		b.byHash[h.Hash()] = h
	}
	return types.CopyHeader(b.headers[number])
}

func (b *witnessBackend) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number == nil {
		if b.advancing {
			b.latest++
		}
		return b.header(b.latest), nil
	}
	b.numberedReads++
	header := b.header(number.Uint64())
	if b.onNumber != nil {
		return b.onNumber(number, b.numberedReads, header)
	}
	return header, nil
}

func (b *witnessBackend) HeaderByHash(_ context.Context, hash common.Hash) (*types.Header, error) {
	b.parentReads++
	h := b.byHash[hash]
	if h == nil {
		return nil, ethereum.NotFound
	}
	if b.onParent != nil {
		return b.onParent(b.parentReads, types.CopyHeader(h))
	}
	return types.CopyHeader(h), nil
}

func (b *witnessBackend) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	b.receiptReads++
	r := b.receipts[hash]
	if b.onReceipt != nil {
		return b.onReceipt(b.receiptReads, r)
	}
	if r == nil {
		return nil, ethereum.NotFound
	}
	return r, nil
}

func TestConfirmationWitnessOldReceiptAdvancingHead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
		b := newWitnessBackend(tx, 10_000)
		b.advancing = true
		m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
		defer cancel()
		got, err := m.waitForConfirmations(ctx, tx.Hash(), b.receipts[tx.Hash()], 2)
		if err != nil || got != b.receipts[tx.Hash()] {
			t.Fatalf("canonical old receipt was not confirmed while latest advanced: %v", err)
		}
		if b.parentReads > 2 || b.numberedReads < 2 || b.receiptReads < 2 {
			t.Fatalf("proof reads: parents=%d numbered=%d receipts=%d", b.parentReads, b.numberedReads, b.receiptReads)
		}
	})
}

func TestConfirmationWitnessDepthBoundedReads(t *testing.T) {
	for _, gap := range []uint64{10, 10_000} {
		for _, depth := range []uint64{0, 2} {
			t.Run(new(big.Int).SetUint64(gap).String()+"/depth-"+new(big.Int).SetUint64(depth).String(), func(t *testing.T) {
				tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
				b := newWitnessBackend(tx, gap)
				m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				receipt := b.receipts[tx.Hash()]
				if err := m.confirmCanonicalReceipt(ctx, receipt); err != nil {
					t.Fatalf("bounded canonical inclusion: %v", err)
				}
				if b.parentReads != 0 {
					t.Fatalf("depth-zero inclusion read %d parents", b.parentReads)
				}
				got, err := m.waitForConfirmations(ctx, tx.Hash(), receipt, depth)
				if err != nil || got != receipt || b.parentReads > int(depth) {
					t.Fatalf("gap %d depth %d: receipt %v err %v parent reads %d", gap, depth, got, err, b.parentReads)
				}
			})
		}
	}
}

func TestConfirmationsRequireStableWitness(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
	b := newWitnessBackend(tx, 2)
	b.onNumber = func(_ *big.Int, calls int, header *types.Header) (*types.Header, error) {
		if calls == 2 {
			header.Extra = []byte("reorg at closing witness fence")
		}
		return header, nil
	}
	m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
	_, err := m.waitForConfirmations(t.Context(), tx.Hash(), b.receipts[tx.Hash()], 2)
	if !errors.Is(err, errReceiptReorged) || b.parentReads != 2 || b.numberedReads != 2 || b.receiptReads != 2 {
		t.Fatalf("witness reorg escaped closing fence: err %v, parents %d, numbered %d, receipts %d", err, b.parentReads, b.numberedReads, b.receiptReads)
	}
}

func TestConfirmationWitnessReorgRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*witnessBackend)
	}{
		{name: "orphan witness", mutate: func(b *witnessBackend) {
			for n := uint64(100); n <= 102; n++ {
				h := forkedReceiptHeader(n, "orphan witness")
				b.byHash[h.Hash()] = h
			}
			b.onNumber = func(number *big.Int, _ int, _ *types.Header) (*types.Header, error) {
				return forkedReceiptHeader(number.Uint64(), "orphan witness"), nil
			}
		}},
		{name: "parent number", mutate: func(b *witnessBackend) {
			b.onParent = func(_ int, h *types.Header) (*types.Header, error) {
				h.Number.Add(h.Number, big.NewInt(1))
				return h, nil
			}
		}},
		{name: "parent hash", mutate: func(b *witnessBackend) {
			b.onParent = func(_ int, h *types.Header) (*types.Header, error) { h.Extra = []byte("wrong parent"); return h, nil }
		}},
		{name: "missing parent", mutate: func(b *witnessBackend) {
			b.onParent = func(_ int, _ *types.Header) (*types.Header, error) { return nil, ethereum.NotFound }
		}},
		{name: "witness number", mutate: func(b *witnessBackend) {
			b.onNumber = func(_ *big.Int, _ int, h *types.Header) (*types.Header, error) {
				h.Number.Add(h.Number, big.NewInt(1))
				return h, nil
			}
		}},
		{name: "witness transport", mutate: func(b *witnessBackend) {
			b.onNumber = func(_ *big.Int, _ int, _ *types.Header) (*types.Header, error) { return nil, io.ErrUnexpectedEOF }
		}},
		{name: "status changed at reread", mutate: func(b *witnessBackend) {
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				fresh := *r
				if n >= 2 {
					fresh.Status = 0
				}
				return &fresh, nil
			}
		}},
		{name: "log identity changed at reread", mutate: func(b *witnessBackend) {
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				fresh := *r
				if n >= 2 {
					fresh.Logs = []*types.Log{{TxHash: common.Hash{1}, BlockHash: r.BlockHash, BlockNumber: r.BlockNumber.Uint64()}}
				}
				return &fresh, nil
			}
		}},
		{name: "receipt hash changed at reread", mutate: func(b *witnessBackend) {
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				fresh := *r
				if n >= 2 {
					fresh.TxHash = common.Hash{1}
				}
				return &fresh, nil
			}
		}},
		{name: "receipt block changed at reread", mutate: func(b *witnessBackend) {
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				fresh := *r
				if n >= 2 {
					fresh.BlockNumber = big.NewInt(101)
					fresh.BlockHash = b.header(101).Hash()
				}
				return &fresh, nil
			}
		}},
		{name: "receipt disappears at reread", mutate: func(b *witnessBackend) {
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				if n >= 2 {
					return nil, ethereum.NotFound
				}
				return r, nil
			}
		}},
		{name: "witness reorg during receipt reread", mutate: func(b *witnessBackend) {
			b.onNumber = func(_ *big.Int, _ int, h *types.Header) (*types.Header, error) {
				if b.receiptReads >= 2 {
					h.Extra = []byte("witness reorg")
				}
				return h, nil
			}
		}},
		{name: "head too shallow", mutate: func(b *witnessBackend) { b.latest = 101 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
				b := newWitnessBackend(tx, 2)
				tc.mutate(b)
				m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
				defer cancel()
				if _, err := m.waitForConfirmations(ctx, tx.Hash(), b.receipts[tx.Hash()], 2); err == nil {
					t.Fatal("invalid or insufficient confirmation proof was accepted")
				}
			})
		})
	}
}

func TestConfirmationWitnessRefreshesReceiptEvidence(t *testing.T) {
	for _, reincluded := range []bool{false, true} {
		name := "same inclusion"
		if reincluded {
			name = "reincluded before opening"
		}
		t.Run(name, func(t *testing.T) {
			tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
			b := newWitnessBackend(tx, 10)
			original := b.receipts[tx.Hash()]
			b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
				fresh := *r
				if reincluded {
					fresh.BlockNumber = big.NewInt(101)
					fresh.BlockHash = b.header(101).Hash()
					fresh.Status = 0
				}
				fresh.Logs = []*types.Log{{TxHash: fresh.TxHash, BlockHash: fresh.BlockHash, BlockNumber: fresh.BlockNumber.Uint64(), Data: []byte{byte(n)}}}
				return &fresh, nil
			}
			m := New(b, mustSigner(t), big.NewInt(1), Config{PollInterval: time.Millisecond}, logr.Discard())
			got, err := m.waitForConfirmations(t.Context(), tx.Hash(), original, 2)
			if err != nil || got.Logs[0].Data[0] != 2 || (reincluded && (got.BlockNumber.Uint64() != 101 || got.Status != 0)) {
				t.Fatalf("confirmation returned stale receipt evidence: %+v, %v", got, err)
			}
		})
	}
}

func TestConfirmationWitnessRejectsInvalidNumbers(t *testing.T) {
	for _, number := range []*big.Int{big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 64), new(big.Int).SetUint64(^uint64(0))} {
		t.Run(number.String(), func(t *testing.T) {
			tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
			b := newWitnessBackend(tx, 2)
			receipt := *b.receipts[tx.Hash()]
			receipt.BlockNumber = number
			m := New(b, mustSigner(t), big.NewInt(1), Config{}, logr.Discard())
			if _, err := m.waitForConfirmations(t.Context(), tx.Hash(), &receipt, 2); err == nil || b.parentReads != 0 {
				t.Fatalf("invalid or overflowing confirmation number accepted: %v", err)
			}
		})
	}
}

func TestLateReceiptWitnessOldReceiptAdvancingHead(t *testing.T) {
	tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
	b := newWitnessBackend(tx, 10_000)
	b.advancing = true
	m := newLateReceiptManager(t, b, Config{Confirmations: 2}, nil)
	got, err := m.readLateReceipt(t.Context(), lateReceiptObservation{
		hash: tx.Hash(), metadata: receiptMetadata{confirmations: 2},
	})
	if err != nil || got.Receipt == nil || got.Outcome != OutcomeConfirmed {
		t.Fatalf("canonical old passive receipt not confirmed while latest advanced: %+v, %v", got, err)
	}
	if b.parentReads > 2 || b.numberedReads < 2 || b.receiptReads < 2 {
		t.Fatalf("passive proof reads: parents=%d numbered=%d receipts=%d", b.parentReads, b.numberedReads, b.receiptReads)
	}
}

func TestLateReceiptWitnessRejectsChangedEvidence(t *testing.T) {
	for _, phase := range []string{"receipt", "closing witness"} {
		t.Run(phase, func(t *testing.T) {
			tx := types.NewTx(&types.DynamicFeeTx{Nonce: 7, To: ptr(common.Address{1})})
			b := newWitnessBackend(tx, 10)
			if phase == "receipt" {
				b.onReceipt = func(n int, r *types.Receipt) (*types.Receipt, error) {
					fresh := *r
					if n >= 2 {
						fresh.Status = types.ReceiptStatusFailed
					}
					return &fresh, nil
				}
			} else {
				b.onNumber = func(_ *big.Int, n int, h *types.Header) (*types.Header, error) {
					if n >= 2 {
						h.Extra = []byte("witness reorg")
					}
					return h, nil
				}
			}
			m := newLateReceiptManager(t, b, Config{Confirmations: 2}, nil)
			got, err := m.readLateReceipt(t.Context(), lateReceiptObservation{
				hash: tx.Hash(), metadata: receiptMetadata{confirmations: 2},
			})
			if err == nil || got.Receipt != nil {
				t.Fatalf("changed passive proof accepted: %+v, %v", got, err)
			}
		})
	}
}
