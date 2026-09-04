package observability

import "sync/atomic"

// Health is the process readiness state. Liveness is tied to the HTTP server itself.
type Health struct {
	ready atomic.Bool
}

func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

func (h *Health) Ready() bool {
	return h.ready.Load()
}
