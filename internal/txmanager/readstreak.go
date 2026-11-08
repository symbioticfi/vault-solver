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
	lastAlertAt time.Time
}

func (s *readStreak) failed(log logr.Logger, err error, msg string, fields ...any) {
	s.failures++
	now := time.Now()
	fields = append(fields, "consecutiveFailures", s.failures)
	switch {
	case s.failures == 1:
		s.since, s.lastAlertAt = now, now
		log.Error(err, msg, fields...)
	case now.Sub(s.lastAlertAt) >= readFailureReminderInterval:
		s.lastAlertAt = now
		log.Error(err, msg, append(fields, "since", now.Sub(s.since).Round(time.Second).String())...)
	default:
		log.V(1).Info(msg, append(fields, "error", err.Error())...)
	}
}

func (s *readStreak) recovered(log logr.Logger, msg string, fields ...any) {
	if s.failures == 0 {
		return
	}
	log.Info(msg, append(fields,
		"consecutiveFailures", s.failures,
		"outage", time.Since(s.since).Round(time.Millisecond).String(),
	)...)
	s.failures = 0
}
