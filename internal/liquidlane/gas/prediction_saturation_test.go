package gas

import "testing"

func TestAdapterPredictionUnitTotalSaturates(t *testing.T) {
	if got := saturatingAddUint64(18_446_744_073_709_551_000, 850_000); got != 18_446_744_073_709_551_615 {
		t.Fatalf("saturating total = %d, want 18446744073709551615", got)
	}
}
