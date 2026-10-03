package liquidlane

import "time"

// SubmissionDeadline translates a deadline expressed in chain time into the wall-clock instant
// after which txmanager must stop submitting the call. chainObservedAt is the wall time immediately
// before chainNow was read; wallNow is sampled immediately before transaction admission. Advancing
// chainNow by elapsed planning time preserves positive chain/wall skew instead of accidentally
// extending the on-chain deadline.
func SubmissionDeadline(
	deadline time.Time,
	chainNow time.Time,
	chainObservedAt time.Time,
	wallNow time.Time,
) (time.Time, bool) {
	if elapsed := wallNow.Sub(chainObservedAt); elapsed > 0 {
		chainNow = chainNow.Add(elapsed)
	}
	reference := chainNow
	if wallNow.After(reference) {
		reference = wallNow
	}
	if !deadline.After(reference) {
		return time.Time{}, false
	}
	return wallNow.Add(deadline.Sub(reference)), true
}
