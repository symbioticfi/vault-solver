package txmanager

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
)

// A second public throwaway key is enough to prove that journal signatures belong to the configured EOA.
const journalValidationOtherKey = "0000000000000000000000000000000000000000000000000000000000000001"

func TestDecodeJournalValidation(t *testing.T) {
	seed, original := journalValidationSeed(t)
	for _, tc := range []struct {
		name    string
		mutate  func(*testing.T, *journalState)
		encode  func([]byte) []byte
		wantErr string
	}{
		{name: "original call"},
		{name: "no request fee limit", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.MaxFeePerGas = "0"
		}},
		{name: "no protocol deadline", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.CancelAt = time.Time{}
		}},
		{name: "expired deadline remains recoverable", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			s.Deadline = time.Now().Add(-time.Hour)
		}},
		{name: "normal replacements and cancellation", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			replacement := journalValidationDynamic(t, s.Attempts[0].Raw)
			replacement.GasFeeCap.Mul(replacement.GasFeeCap, big.NewInt(2))
			s.Attempts = append(s.Attempts, journalAttempt{Raw: journalValidationSign(t, types.NewTx(replacement), "")})
			s.Attempts = append(s.Attempts, journalValidationCancellation(t, seed, s))
			s.CancelReason = "shutdown"
		}},
		{name: "malformed JSON", encode: func([]byte) []byte { return []byte(`{"version":`) }, wantErr: "decode state"},
		{name: "trailing state", encode: func(data []byte) []byte { return append(data, []byte(` {}`)...) }, wantErr: "exactly one state"},
		{name: "trailing malformed bytes", encode: func(data []byte) []byte { return append(data, 'x') }, wantErr: "exactly one state"},
		{name: "unknown field", encode: func(data []byte) []byte {
			return append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
		}, wantErr: "unknown field"},
		{name: "unknown request field", encode: func(data []byte) []byte {
			return []byte(strings.Replace(string(data), `"request":{`, `"request":{"unknown":true,`, 1))
		}, wantErr: "unknown field"},
		{name: "unsupported version", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Version++
		}, wantErr: "version, chain or sender mismatch"},
		{name: "missing version", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Version = 0
		}, wantErr: "version, chain or sender mismatch"},
		{name: "different chain", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.ChainID = "1"
		}, wantErr: "version, chain or sender mismatch"},
		{name: "different sender", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Sender = common.HexToAddress("0x123")
		}, wantErr: "version, chain or sender mismatch"},
		{name: "no signed attempts", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Attempts = nil
		}, wantErr: "original signed call"},
		{name: "no cancellation deadline", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Deadline = time.Time{}
		}, wantErr: "cancellation deadline"},
		{name: "cancellation first", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Attempts[0].Cancellation = true
		}, wantErr: "original signed call"},
		{name: "extended request deadline", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Deadline = s.Request.CancelAt.Add(time.Second)
		}, wantErr: "exceeds request deadline"},
		{name: "invalid value", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.Value = "missing"
		}, wantErr: "journal integer"},
		{name: "negative value", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.Value = "-1"
		}, wantErr: "journal integer"},
		{name: "invalid base fee", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.BaseFee = "1.5"
		}, wantErr: "journal integer"},
		{name: "negative base fee", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.BaseFee = "-1"
		}, wantErr: "journal integer"},
		{name: "invalid fee limit", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.MaxFeePerGas = "invalid"
		}, wantErr: "journal integer"},
		{name: "negative fee limit", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.MaxFeePerGas = "-1"
		}, wantErr: "journal integer"},
		{name: "invalid cancellation reason", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.CancelReason = "unknown"
		}, wantErr: "cancellation reason"},
		{name: "malformed signed bytes", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Attempts[0].Raw = []byte{0xff}
		}, wantErr: "decode signed attempt"},
		{name: "unsigned transaction", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.V, tx.R, tx.S = nil, nil, nil
			s.Attempts[0].Raw = journalValidationRaw(t, types.NewTx(tx))
		}, wantErr: "sender mismatch"},
		{name: "signed by different sender", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), journalValidationOtherKey)
		}, wantErr: "sender mismatch"},
		{name: "signed on different chain", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.ChainID = big.NewInt(1)
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "type, chain or nonce mismatch"},
		{name: "different normal destination", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.To = common.HexToAddress("0xdef")
		}, wantErr: "differs from journal request"},
		{name: "different normal calldata", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.Data = []byte{0xff}
		}, wantErr: "differs from journal request"},
		{name: "different normal value", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Request.Value = "6"
		}, wantErr: "differs from journal request"},
		{name: "replacement at different nonce", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.Nonce++
			s.Attempts = append(s.Attempts, journalAttempt{Raw: journalValidationSign(t, types.NewTx(tx), "")})
		}, wantErr: "type, chain or nonce mismatch"},
		{name: "overflowing next nonce", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.Nonce = math.MaxUint64
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "type, chain or nonce mismatch"},
		{name: "duplicate attempt", mutate: func(t *testing.T, s *journalState) {
			t.Helper()
			s.Attempts = append(s.Attempts, s.Attempts[0])
		}, wantErr: "duplicate attempt"},
		{name: "normal call after cancellation", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			cancel := journalValidationCancellation(t, seed, s)
			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.GasFeeCap.Mul(tx.GasFeeCap, big.NewInt(2))
			s.Attempts = append(s.Attempts, cancel, journalAttempt{Raw: journalValidationSign(t, types.NewTx(tx), "")})
		}, wantErr: "normal call after cancellation"},
		{name: "contract creation", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.To = nil
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "invalid signed call or fees"},
		{name: "zero gas", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.Gas = 0
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "invalid signed call or fees"},
		{name: "zero fee cap", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.GasFeeCap.SetInt64(0)
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "invalid signed call or fees"},
		{name: "tip exceeds fee cap", mutate: func(t *testing.T, s *journalState) {
			t.Helper()

			tx := journalValidationDynamic(t, s.Attempts[0].Raw)
			tx.GasTipCap.Add(tx.GasFeeCap, big.NewInt(1))
			s.Attempts[0].Raw = journalValidationSign(t, types.NewTx(tx), "")
		}, wantErr: "invalid signed call or fees"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := journalValidationClone(t, original)
			if tc.mutate != nil {
				tc.mutate(t, &state)
			}
			data := journalValidationEncode(t, state)
			if tc.encode != nil {
				data = tc.encode(data)
			}
			pending, err := seed.decodeJournal(data)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || pending != nil {
					t.Fatalf("decode = %+v, %v; want error containing %q", pending, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !pending.recovered || pending.req.Obsolete != nil || len(pending.attempts) != len(state.Attempts) ||
				!pending.cancelDeadline.Equal(state.Deadline) || pending.req.Label != state.Request.Label ||
				pending.req.Solver != state.Request.Solver {
				t.Fatalf("recovered metadata = %+v", pending)
			}
		})
	}
}

func TestDecodeJournalCancellationValidation(t *testing.T) {
	seed, original := journalValidationSeed(t)
	for _, tc := range []struct {
		name    string
		mutate  func(*types.DynamicFeeTx)
		key     string
		legacy  bool
		wantErr string
	}{
		{name: "valid cancellation"},
		{name: "different cancellation destination", mutate: func(tx *types.DynamicFeeTx) {
			to := common.HexToAddress("0xdef")
			tx.To = &to
		}, wantErr: "invalid signed cancellation"},
		{name: "nonzero cancellation value", mutate: func(tx *types.DynamicFeeTx) { tx.Value = big.NewInt(1) }, wantErr: "invalid signed cancellation"},
		{name: "cancellation calldata", mutate: func(tx *types.DynamicFeeTx) { tx.Data = []byte{1} }, wantErr: "invalid signed cancellation"},
		{name: "different cancellation gas", mutate: func(tx *types.DynamicFeeTx) { tx.Gas++ }, wantErr: "invalid signed cancellation"},
		{name: "different cancellation signer", key: journalValidationOtherKey, wantErr: "sender mismatch"},
		{name: "legacy cancellation", legacy: true, wantErr: "type, chain or nonce mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := journalValidationClone(t, original)
			attempt := journalValidationCancellation(t, seed, &state)
			tx := journalValidationDynamic(t, attempt.Raw)
			if tc.mutate != nil {
				tc.mutate(tx)
			}
			unsigned := types.NewTx(tx)
			if tc.legacy {
				unsigned = types.NewTx(&types.LegacyTx{Nonce: tx.Nonce, To: tx.To, Gas: tx.Gas, GasPrice: tx.GasFeeCap})
			}
			attempt.Raw = journalValidationSign(t, unsigned, tc.key)
			state.Attempts = append(state.Attempts, attempt)
			pending, err := seed.decodeJournal(journalValidationEncode(t, state))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("decode error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || !pending.latestAttempt().cancellation {
				t.Fatalf("decode cancellation = %+v, %v", pending, err)
			}
		})
	}
}

func TestInitializeJournalAccountNonceValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		latest  uint64
		pending uint64
		wantErr bool
	}{
		{name: "hidden original", latest: 7, pending: 7},
		{name: "visible original", latest: 7, pending: 8},
		{name: "owned nonce already mined", latest: 8, pending: 8},
		{name: "unknown older gap", latest: 6, pending: 7, wantErr: true},
		{name: "unexpected future latest", latest: 9, pending: 9, wantErr: true},
		{name: "unexpected future pending", latest: 7, pending: 9, wantErr: true},
		{name: "pending behind latest", latest: 7, pending: 6, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed, _ := journalValidationSeed(t)
			backend := newMockBackend()
			backend.latestNonce, backend.pendingNonce = tc.latest, tc.pending
			m := New(backend, seed.signer, seed.chainID, seed.cfg, logr.Discard())
			t.Cleanup(func() {
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			})
			err := m.Initialize(t.Context())
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "cannot own account") {
					t.Fatalf("Initialize error = %v, want unknown ownership error", err)
				}
				// The failed manager must release its lock so a corrected startup can take ownership.
				corrected := New(newMockBackend(), seed.signer, seed.chainID, seed.cfg, logr.Discard())
				if err := corrected.Initialize(t.Context()); err != nil {
					t.Fatalf("corrected startup retained previous lock: %v", err)
				}
				if err := corrected.Close(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil || m.Available() || m.LaneReady() || m.recovered == nil || m.recovered.nonce != 7 || m.nonce != 8 {
				t.Fatalf("Initialize = %v; available=%t ready=%t recovered=%+v nextNonce=%d", err, m.Available(), m.LaneReady(), m.recovered, m.nonce)
			}
		})
	}
}

func TestInitializeInvalidJournalReleasesLock(t *testing.T) {
	seed, state := journalValidationSeed(t)
	valid := journalValidationEncode(t, state)
	for _, invalid := range [][]byte{[]byte(`{"version":`), []byte(`{}`), nil} {
		if err := os.WriteFile(seed.cfg.StateFile, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		m := New(newMockBackend(), seed.signer, seed.chainID, seed.cfg, logr.Discard())
		if err := m.Initialize(t.Context()); err == nil {
			t.Fatal("invalid journal initialized")
		}
		if err := os.WriteFile(seed.cfg.StateFile, valid, 0600); err != nil {
			t.Fatal(err)
		}
		// Retry the same manager after correcting the journal; failed initialization must be clean.
		if err := m.Initialize(t.Context()); err != nil {
			t.Fatalf("corrected journal cannot initialize: %v", err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInitializeJournalPreservesSavedPolicy(t *testing.T) {
	for _, confirmations := range []uint64{0, 3} {
		t.Run(strconv.FormatUint(confirmations, 10), func(t *testing.T) {
			seed, state := journalValidationSeed(t)
			state.Request.Confirmations = confirmations
			state.CancelReason, state.Obsolete = "obsolete", true
			if err := os.WriteFile(seed.cfg.StateFile, journalValidationEncode(t, state), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := seed.cfg
			cfg.Confirmations, cfg.PendingTimeout = 99, 10*time.Hour
			m := New(newMockBackend(), seed.signer, seed.chainID, cfg, logr.Discard())
			if err := m.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			})
			pending := m.recovered
			if m.confirmations(pending.req) != confirmations || !pending.cancelDeadline.Equal(state.Deadline) ||
				pending.cancelReason != "obsolete" || !pending.obsolete || pending.req.Obsolete != nil ||
				pending.req.MaxFeePerGas.String() != state.Request.MaxFeePerGas {
				t.Fatalf("saved policy changed: %+v", pending)
			}
		})
	}
}

func journalValidationSeed(t *testing.T) (*Manager, journalState) {
	t.Helper()
	s, err := signer.NewFromHexKey(testKey)
	if err != nil {
		t.Fatal(err)
	}
	m := New(newMockBackend(), s, big.NewInt(11155111), Config{
		StateFile: filepath.Join(t.TempDir(), "transactions.json"), Confirmations: 3, MaxFeeGwei: 100,
	}, logr.Discard())
	if err := m.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := m.broadcast(t.Context(), Request{
		To: common.HexToAddress("0xabc"), Data: []byte{1, 2}, Value: big.NewInt(5), GasLimit: 50_000,
		MaxFeePerGas: big.NewInt(60_000_000_000), CancelAt: time.Now().Add(time.Hour), Label: "validate", Solver: "test",
	}); err != nil {
		t.Fatal(err)
	}
	data, err := m.journal.load()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	var state journalState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return m, state
}

func journalValidationClone(t *testing.T, state journalState) journalState {
	t.Helper()
	var cloned journalState
	if err := json.Unmarshal(journalValidationEncode(t, state), &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func journalValidationEncode(t *testing.T, state journalState) []byte {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func journalValidationDynamic(t *testing.T, raw []byte) *types.DynamicFeeTx {
	t.Helper()
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		t.Fatal(err)
	}
	return &types.DynamicFeeTx{
		ChainID: tx.ChainId(), Nonce: tx.Nonce(), To: tx.To(), Data: tx.Data(), Value: tx.Value(),
		Gas: tx.Gas(), GasFeeCap: tx.GasFeeCap(), GasTipCap: tx.GasTipCap(),
	}
}

func journalValidationSign(t *testing.T, tx *types.Transaction, key string) []byte {
	t.Helper()
	if key == "" {
		key = testKey
	}
	s, err := signer.NewFromHexKey(key)
	if err != nil {
		t.Fatal(err)
	}
	chainID := tx.ChainId()
	if tx.Type() == types.LegacyTxType {
		chainID = big.NewInt(11155111)
	}
	signed, err := s.SignTx(t.Context(), tx, chainID)
	if err != nil {
		t.Fatal(err)
	}
	return journalValidationRaw(t, signed)
}

func journalValidationRaw(t *testing.T, tx *types.Transaction) []byte {
	t.Helper()
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func journalValidationCancellation(t *testing.T, seed *Manager, state *journalState) journalAttempt {
	t.Helper()
	tx := journalValidationDynamic(t, state.Attempts[0].Raw)
	to := seed.signer.Address()
	tx.To, tx.Data, tx.Value, tx.Gas = &to, nil, new(big.Int), cancellationGasLimit
	tx.GasFeeCap.Mul(tx.GasFeeCap, big.NewInt(2))
	return journalAttempt{Raw: journalValidationSign(t, types.NewTx(tx), ""), Cancellation: true}
}
