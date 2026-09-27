package handler

import (
	"sync"

	"github.com/githoober/monogo"
)

// Test records handled logs in memory for assertions during testing.
type Test struct {
	BaseHandler
	records []monogo.Record
	mu      sync.RWMutex
}

// NewTest creates a Test handler with optional configuration options.
func NewTest(level monogo.Level, opts ...Option) *Test {
	return &Test{
		BaseHandler: NewBaseHandler(level, opts...),
		records:     make([]monogo.Record, 0),
	}
}

// Handle stores the log record in memory.
func (t *Test) Handle(record monogo.Record) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, record)
	return nil
}

// HandleBatch stores all handled log records in memory.
func (t *Test) HandleBatch(records []monogo.Record) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, rec := range records {
		if t.IsHandling(rec.Level) {
			t.records = append(t.records, rec)
		}
	}
	return nil
}

// Records returns a slice copy of logged records.
func (t *Test) Records() []monogo.Record {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]monogo.Record, len(t.records))
	copy(cp, t.records)
	return cp
}

// HasRecord checks if any record matches predicate.
func (t *Test) HasRecord(predicate func(monogo.Record) bool) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, r := range t.records {
		if predicate(r) {
			return true
		}
	}
	return false
}

// Reset clears recorded records.
func (t *Test) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make([]monogo.Record, 0)
}

// Close resets the handler.
func (t *Test) Close() error {
	t.Reset()
	return nil
}

// Null discards all records.
type Null struct {
	BaseHandler
}

// NewNull creates a Null handler with optional configuration options.
func NewNull(opts ...Option) *Null {
	return &Null{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
	}
}

// Handle does nothing.
func (n *Null) Handle(record monogo.Record) error {
	return nil
}

// Close does nothing.
func (n *Null) Close() error {
	return nil
}
