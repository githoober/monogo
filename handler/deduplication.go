package handler

import (
	"context"
	"sync"
	"time"

	"github.com/githoober/monogo"
)

// DeduplicationStore defines the storage contract for checking and updating deduplication records.
type DeduplicationStore interface {
	// IsDuplicate reports whether key was already seen within window, and records now as its timestamp if new.
	IsDuplicate(key string, now time.Time, window time.Duration) bool
	// Reset clears all deduplication entries from storage.
	Reset()
}

type memoryStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
	ops     int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		entries: make(map[string]time.Time),
	}
}

func (s *memoryStore) IsDuplicate(key string, now time.Time, window time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ops++
	if s.ops > 128 {
		s.pruneLocked(now, window)
		s.ops = 0
	}

	lastSeen, exists := s.entries[key]
	if exists && now.Sub(lastSeen) < window {
		return true
	}

	s.entries[key] = now
	return false
}

func (s *memoryStore) pruneLocked(now time.Time, window time.Duration) {
	for k, t := range s.entries {
		if now.Sub(t) >= window {
			delete(s.entries, k)
		}
	}
}

func (s *memoryStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]time.Time)
	s.ops = 0
}

// WithDeduplicationKey configures a custom key extractor for identifying duplicate records.
func WithDeduplicationKey(fn func(monogo.Record) string) Option {
	return func(o *options) {
		o.dedupKeyFunc = fn
	}
}

// WithDeduplicationStore configures a custom DeduplicationStore.
func WithDeduplicationStore(store DeduplicationStore) Option {
	return func(o *options) {
		o.dedupStore = store
	}
}

// Deduplication buffers or filters log records to suppress duplicate messages occurring
// within a specified time window.
// Records below dedupLevel pass through unconditionally; records at or above dedupLevel
// are deduplicated according to the configured window.
type Deduplication struct {
	BaseHandler
	handler    monogo.Handler
	dedupLevel monogo.Level
	timeWindow time.Duration
	keyFunc    func(monogo.Record) string
	store      DeduplicationStore
}

// DeduplicationHandler is an alias for Deduplication.
type DeduplicationHandler = Deduplication

// Compile-time interface checks.
var (
	_ monogo.Handler            = (*Deduplication)(nil)
	_ monogo.BatchHandler       = (*Deduplication)(nil)
	_ monogo.Bubbler            = (*Deduplication)(nil)
	_ monogo.ProcessableHandler = (*Deduplication)(nil)
)

func defaultKeyFunc(r monogo.Record) string {
	return r.Level.String() + ":" + r.Channel + ":" + r.Message
}

// NewDeduplication creates a Deduplication handler wrapping the given handler.
// Records with level >= dedupLevel are deduplicated across timeWindow.
// Records with level < dedupLevel pass through directly without deduplication.
func NewDeduplication(handler monogo.Handler, dedupLevel monogo.Level, timeWindow time.Duration, opts ...Option) *Deduplication {
	if dedupLevel == 0 {
		dedupLevel = monogo.ERROR
	}
	if timeWindow <= 0 {
		timeWindow = 60 * time.Second
	}

	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	keyFn := o.dedupKeyFunc
	if keyFn == nil {
		keyFn = defaultKeyFunc
	}

	store := o.dedupStore
	if store == nil {
		store = newMemoryStore()
	}

	return &Deduplication{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handler:     handler,
		dedupLevel:  dedupLevel,
		timeWindow:  timeWindow,
		keyFunc:     keyFn,
		store:       store,
	}
}

// IsHandling checks if the wrapped handler handles the log level.
func (d *Deduplication) IsHandling(ctx context.Context, level monogo.Level) bool {
	return d.handler.IsHandling(ctx, level)
}

// Handle processes a single log record.
// If record level >= dedupLevel and it is a duplicate within the time window,
// it is suppressed (returning nil). Otherwise it is forwarded to the inner handler.
func (d *Deduplication) Handle(ctx context.Context, record monogo.Record) error {
	record = d.ProcessRecord(record)

	if record.Level >= d.dedupLevel {
		key := d.keyFunc(record)
		now := record.Time
		if now.IsZero() {
			now = time.Now()
		}
		if d.store.IsDuplicate(key, now, d.timeWindow) {
			return nil
		}
	}

	return d.handler.Handle(ctx, record)
}

// HandleBatch processes a batch of records, filtering out duplicate records within the batch
// and against recent history before passing the deduplicated batch to the inner handler.
func (d *Deduplication) HandleBatch(ctx context.Context, records []monogo.Record) error {
	if len(d.processors) > 0 {
		processed := make([]monogo.Record, len(records))
		for i, rec := range records {
			processed[i] = d.ProcessRecord(rec)
		}
		records = processed
	}

	deduped := make([]monogo.Record, 0, len(records))
	for _, rec := range records {
		if rec.Level >= d.dedupLevel {
			key := d.keyFunc(rec)
			now := rec.Time
			if now.IsZero() {
				now = time.Now()
			}
			if d.store.IsDuplicate(key, now, d.timeWindow) {
				continue
			}
		}
		deduped = append(deduped, rec)
	}

	if len(deduped) == 0 {
		return nil
	}

	if bh, ok := d.handler.(monogo.BatchHandler); ok {
		return bh.HandleBatch(ctx, deduped)
	}

	var lastErr error
	for _, rec := range deduped {
		if err := d.handler.Handle(ctx, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Reset clears the deduplication store.
func (d *Deduplication) Reset() {
	d.store.Reset()
}

// Close closes the wrapped handler.
func (d *Deduplication) Close(ctx context.Context) error {
	return d.handler.Close(ctx)
}
