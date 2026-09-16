package observability

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const defaultSpanLinkEntries = 1024

// SpanLinks remembers span contexts by an application key (quote id, auction id, request address)
// so later work in a different context tree can link back to them (spec §12). It is process-local,
// bounded, and best effort: a miss is an ordinary (link, false) result, never an error.
type SpanLinks struct {
	mu      sync.Mutex
	max     int
	now     func() time.Time
	entries map[string]spanLinkEntry
	order   []string // insertion order for eviction
}

type spanLinkEntry struct {
	sc      trace.SpanContext
	expires time.Time
}

// NewSpanLinks creates a map holding at most maxEntries (1024 when <= 0).
func NewSpanLinks(maxEntries int) *SpanLinks {
	if maxEntries <= 0 {
		maxEntries = defaultSpanLinkEntries
	}
	return &SpanLinks{max: maxEntries, now: time.Now, entries: make(map[string]spanLinkEntry, maxEntries)}
}

// Remember stores the span context of ctx under key for ttl. Empty keys and contexts without a
// valid span context are ignored, so callers never need to check tracing state first.
func (l *SpanLinks) Remember(ctx context.Context, key string, ttl time.Duration) {
	sc := trace.SpanContextFromContext(ctx)
	key = normalizeLinkKey(key)
	if key == "" || !sc.IsValid() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.entries[key]; !exists {
		l.order = append(l.order, key)
	}
	l.entries[key] = spanLinkEntry{sc: sc, expires: l.now().Add(ttl)}
	for len(l.order) > l.max {
		oldest := l.order[0]
		l.order = l.order[1:]
		delete(l.entries, oldest)
	}
}

// Lookup returns a link to the remembered span, dropping it if expired.
func (l *SpanLinks) Lookup(key string) (trace.Link, bool) {
	key = normalizeLinkKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return trace.Link{}, false
	}
	if !l.now().Before(entry.expires) {
		delete(l.entries, key)
		return trace.Link{}, false
	}
	return trace.Link{SpanContext: entry.sc}, true
}

// Len reports stored entries, expired or not (for tests and diagnostics).
func (l *SpanLinks) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func normalizeLinkKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }
