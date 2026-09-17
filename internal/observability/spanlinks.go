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
// bounded, and best effort: a miss is an ordinary (link, false) result, never an error. A nil
// *SpanLinks disables linking, so callers never need to guard on it.
type SpanLinks struct {
	mu      sync.Mutex
	max     int
	now     func() time.Time
	nextSeq uint64
	entries map[string]spanLinkEntry
	order   []orderedKey // insertion order for eviction; may hold entries stale by seq
}

type spanLinkEntry struct {
	sc      trace.SpanContext
	expires time.Time
	seq     uint64 // matched against order to detect a stale (re-remembered or expired) slot
}

type orderedKey struct {
	key string
	seq uint64
}

// NewSpanLinks creates a map holding at most defaultSpanLinkEntries entries.
func NewSpanLinks() *SpanLinks {
	return newSpanLinks(defaultSpanLinkEntries)
}

func newSpanLinks(maxEntries int) *SpanLinks {
	return &SpanLinks{max: maxEntries, now: time.Now, entries: make(map[string]spanLinkEntry, maxEntries)}
}

// Remember stores the span context of ctx under key for ttl. Empty keys and contexts without a
// valid span context are ignored, so callers never need to check tracing state first.
func (l *SpanLinks) Remember(ctx context.Context, key string, ttl time.Duration) {
	if l == nil {
		return
	}
	sc := trace.SpanContextFromContext(ctx)
	key = normalizeLinkKey(key)
	if key == "" || !sc.IsValid() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextSeq++
	seq := l.nextSeq
	l.entries[key] = spanLinkEntry{sc: sc, expires: l.now().Add(ttl), seq: seq}
	l.order = append(l.order, orderedKey{key: key, seq: seq})
	for len(l.order) > l.max {
		oldest := l.order[0]
		l.order = l.order[1:]
		// The popped slot is stale (the key was re-remembered or expired since) when its seq no
		// longer matches the live entry; only a still-matching slot may evict the key it names.
		if e, ok := l.entries[oldest.key]; ok && e.seq == oldest.seq {
			delete(l.entries, oldest.key)
		}
	}
}

// Lookup returns a link to the remembered span, dropping it if expired.
func (l *SpanLinks) Lookup(key string) (trace.Link, bool) {
	if l == nil {
		return trace.Link{}, false
	}
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

// len reports stored entries, expired or not (for tests and diagnostics).
func (l *SpanLinks) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func normalizeLinkKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }
