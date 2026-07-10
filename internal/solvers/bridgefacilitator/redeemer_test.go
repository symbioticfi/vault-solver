package bridgefacilitator

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestPendingRedemptions_SuppressUntilAuthoritativeAbsence(t *testing.T) {
	adapter := common.HexToAddress("0xa")
	r1 := common.HexToAddress("0x1")
	r2 := common.HexToAddress("0x2")
	s := &Solver{pendingRedemptions: make(map[redeemKey]struct{})}

	s.recordPendingRedemptions(adapter, []common.Address{r1, r2})
	got := s.filterPendingRedemptions(adapter, []common.Address{r1, r2})
	if len(got) != 0 {
		t.Fatalf("unresolved requests were offered again: %v", got)
	}

	// An authoritative scan no longer containing r1 clears only r1.
	s.reconcilePendingRedemptions(adapter, []common.Address{r2})
	got = s.filterPendingRedemptions(adapter, []common.Address{r1, r2})
	if len(got) != 1 || got[0] != r1 {
		t.Fatalf("filtered = %v, want only %s retryable after authoritative absence", got, r1)
	}
}

func TestPendingRedemptions_EmptyScanClearsOnlyItsAdapter(t *testing.T) {
	adapterA := common.HexToAddress("0xa")
	adapterB := common.HexToAddress("0xb")
	r1 := common.HexToAddress("0x1")
	r2 := common.HexToAddress("0x2")
	s := &Solver{pendingRedemptions: make(map[redeemKey]struct{})}
	s.recordPendingRedemptions(adapterA, []common.Address{r1})
	s.recordPendingRedemptions(adapterB, []common.Address{r2})

	ready, err := s.reconcileReadyRedemptions(adapterA, nil, nil)
	if err != nil || len(ready) != 0 {
		t.Fatalf("empty scan result = %v, %v; want empty success", ready, err)
	}
	if got := s.filterPendingRedemptions(adapterA, []common.Address{r1}); len(got) != 1 || got[0] != r1 {
		t.Fatalf("adapter A filtered = %v, want request retryable after empty authoritative scan", got)
	}
	if got := s.filterPendingRedemptions(adapterB, []common.Address{r2}); len(got) != 0 {
		t.Fatalf("adapter B pending request was cleared by adapter A scan: %v", got)
	}
}

func TestPendingRedemptions_ReadErrorPreservesSuppression(t *testing.T) {
	adapter := common.HexToAddress("0xa")
	request := common.HexToAddress("0x1")
	s := &Solver{pendingRedemptions: make(map[redeemKey]struct{})}
	s.recordPendingRedemptions(adapter, []common.Address{request})
	wantErr := errors.New("scan failed")

	ready, err := s.reconcileReadyRedemptions(adapter, nil, wantErr)
	if !errors.Is(err, wantErr) || ready != nil {
		t.Fatalf("read error result = %v, %v; want nil, %v", ready, err, wantErr)
	}
	if got := s.filterPendingRedemptions(adapter, []common.Address{request}); len(got) != 0 {
		t.Fatalf("read error cleared pending suppression: %v", got)
	}
}

func TestRedeemResult_UnresolvedRecordsWholeBatch(t *testing.T) {
	adapter := common.HexToAddress("0xa")
	batch := []common.Address{common.HexToAddress("0x1"), common.HexToAddress("0x2")}
	s := &Solver{pendingRedemptions: make(map[redeemKey]struct{}), log: logr.Discard()}
	s.handleRedeemResult(adapter, batch, txmanager.Result{
		State: txmanager.StateUnresolved, Err: txmanager.ErrUnresolved,
	})
	if got := s.filterPendingRedemptions(adapter, batch); len(got) != 0 {
		t.Fatalf("unresolved batch not suppressed: %v", got)
	}
}

func TestRedeemResult_IntermediateStatesSuppressWholeBatch(t *testing.T) {
	for _, state := range []txmanager.State{
		txmanager.StateBroadcastUnknown,
		txmanager.StatePending,
		txmanager.State("future_state"),
	} {
		t.Run(string(state), func(t *testing.T) {
			adapter := common.HexToAddress("0xa")
			batch := []common.Address{common.HexToAddress("0x1"), common.HexToAddress("0x2")}
			s := &Solver{pendingRedemptions: make(map[redeemKey]struct{}), log: logr.Discard()}

			s.handleRedeemResult(adapter, batch, txmanager.Result{State: state, Err: errors.New("intermediate")})

			if got := s.filterPendingRedemptions(adapter, batch); len(got) != 0 {
				t.Fatalf("state %q did not suppress whole batch: %v", state, got)
			}
		})
	}
}

func TestRedeemResult_DefiniteFailuresRemainRetryable(t *testing.T) {
	for _, state := range []txmanager.State{
		txmanager.StateNotBroadcast,
		txmanager.StateRejected,
		txmanager.StateReverted,
	} {
		t.Run(string(state), func(t *testing.T) {
			adapter := common.HexToAddress("0xa")
			batch := []common.Address{common.HexToAddress("0x1"), common.HexToAddress("0x2")}
			s := &Solver{pendingRedemptions: make(map[redeemKey]struct{}), log: logr.Discard()}

			s.handleRedeemResult(adapter, batch, txmanager.Result{State: state, Err: errors.New("definite failure")})

			if got := s.filterPendingRedemptions(adapter, batch); len(got) != len(batch) {
				t.Fatalf("state %q suppressed retryable batch: %v", state, got)
			}
		})
	}
}
