package types

import (
	"testing"

	"github.com/go-errors/errors"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

func TestPermanentFillDecisionError(t *testing.T) {
	cause := errors.New("unsupported order context")
	marked := MarkPermanentFillDecisionError(cause)

	if !IsPermanentFillDecisionError(marked) {
		t.Fatal("marked error was not classified as permanent")
	}
	if !errors.Is(marked, cause) {
		t.Fatal("marked error does not preserve its cause")
	}
	if got := MarkPermanentFillDecisionError(marked); !IsPermanentFillDecisionError(got) || !errors.Is(got, cause) {
		t.Fatal("marking a permanent error twice lost its classification or cause")
	}
	testcheck.NoError(t, MarkPermanentFillDecisionError(nil), "MarkPermanentFillDecisionError(nil) = %v")
}
