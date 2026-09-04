package liquidlane

import (
	"testing"
	"time"
)

func TestCancellationDeadline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		deadline        int64
		chainNow        int64
		chainObservedAt int64
		wallNow         int64
		want            int64
		wantOK          bool
	}{
		{
			name:     "chain clock ahead",
			deadline: 1_030, chainNow: 1_010, chainObservedAt: 1_000, wallNow: 1_000,
			want: 1_020, wantOK: true,
		},
		{
			name:     "wall clock ahead",
			deadline: 1_030, chainNow: 1_000, chainObservedAt: 1_010, wallNow: 1_010,
			want: 1_030, wantOK: true,
		},
		{
			name:     "planning latency preserves chain skew",
			deadline: 1_030, chainNow: 1_010, chainObservedAt: 1_000, wallNow: 1_015,
			want: 1_020, wantOK: true,
		},
		{
			name:     "deadline reached at observation",
			deadline: 1_010, chainNow: 1_010, chainObservedAt: 1_000, wallNow: 1_000,
			wantOK: false,
		},
		{
			name:     "deadline reached during planning",
			deadline: 1_030, chainNow: 1_010, chainObservedAt: 1_000, wallNow: 1_020,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := CancellationDeadline(
				time.Unix(tt.deadline, 0),
				time.Unix(tt.chainNow, 0),
				time.Unix(tt.chainObservedAt, 0),
				time.Unix(tt.wallNow, 0),
			)
			if ok != tt.wantOK {
				t.Fatalf("valid = %v, want %v (deadline %s)", ok, tt.wantOK, got)
			}
			if ok && got.Unix() != tt.want {
				t.Fatalf("deadline = %d, want %d", got.Unix(), tt.want)
			}
		})
	}
}
