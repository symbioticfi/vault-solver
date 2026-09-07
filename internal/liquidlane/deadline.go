package liquidlane

import "time"

// CancellationDeadline translates a deadline expressed in chain time into the wall-clock instant
// expected by txmanager. chainObservedAt is the wall time immediately before chainNow was read;
// wallNow is sampled immediately before transaction admission. Advancing chainNow by elapsed planning
// time preserves positive chain/wall skew instead of accidentally extending the on-chain deadline.
func CancellationDeadline(deadline, chainNow, observedAt, wallNow time.Time) (time.Time, bool) {
	chainAtAdmission := chainNow.Add(max(wallNow.Sub(observedAt), 0))
	lifetime := min(deadline.Sub(chainAtAdmission), deadline.Sub(wallNow))
	if lifetime <= 0 {
		return time.Time{}, false
	}
	return wallNow.Add(lifetime), true
}
