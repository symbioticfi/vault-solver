package txmanager

import (
	"time"

	"github.com/go-logr/logr"
)

// readFailureReminderInterval is how often an unrecovered read streak is re-raised at error
// level. A variable so tests can shorten it.
var readFailureReminderInterval = 5 * time.Minute

// readStreak collapses a run of failed reads of one resource into one error when the run starts,
// debug lines while it lasts, an error reminder every readFailureReminderInterval, and one info
// line when reads recover, so a stuck RPC does not log an error on every poll.
type readStreak struct {
	failures    int
	since       time.Time
	nextAlertAt time.Time
}

func (s *readStreak) failed(log logr.Logger, err error, message string, fields ...any) {
	now := time.Now()
	first := s.failures == 0
	if first {
		s.since = now
	}
	s.failures++
	log = log.WithValues(fields...).WithValues("consecutiveFailures", s.failures)
	if first || !now.Before(s.nextAlertAt) {
		s.nextAlertAt = now.Add(readFailureReminderInterval)
		if !first {
			log = log.WithValues("since", now.Sub(s.since).Round(time.Second).String())
		}
		log.Error(err, message)
	} else {
		log.V(1).Info(message, "error", err.Error())
	}
}

func (s *readStreak) recovered(log logr.Logger, message string, fields ...any) {
	if s.failures == 0 {
		return
	}
	log.WithValues(fields...).Info(message, "consecutiveFailures", s.failures, "outage", time.Since(s.since).Round(time.Millisecond).String())
	*s = readStreak{}
}
