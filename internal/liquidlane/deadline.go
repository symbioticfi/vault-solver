package liquidlane

import "time"

// CancellationDeadline projects a chain deadline onto wall time without extending it under skew.
func CancellationDeadline(deadline, chainNow, chainObservedAt, wallNow time.Time) (time.Time, bool) {
	if planningTime := wallNow.Sub(chainObservedAt); planningTime > 0 {
		chainNow = chainNow.Add(planningTime)
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
