package handler

import (
	"sync"

	"github.com/githoober/monogo"
)

// Test records handled logs in memory for assertions during testing.
type Test struct {
	BaseHandler
	records []monolog.Record
	mu      sync.RWMutex
}

// NewTest creates a Test handler with optional bubbling control (defaults to true).
func NewTest(level monolog.Level, bubble ...bool) *Test {
	return &Test{
		BaseHandler: NewBaseHandler(level, bubble...),
		records:     make([]monolog.Record, 0),
	}
}

// Handle stores the log record in memory.
func (t *Test) Handle(record monolog.Record) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, record)
	return nil
}

// Records returns a slice copy of logged records.
func (t *Test) Records() []monolog.Record {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]monolog.Record, len(t.records))
	copy(cp, t.records)
	return cp
}

// HasRecord checks if any record matches predicate.
func (t *Test) HasRecord(predicate func(monolog.Record) bool) bool {
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
	t.records = make([]monolog.Record, 0)
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

// NewNull creates a Null handler with optional bubbling control (defaults to true).
func NewNull(bubble ...bool) *Null {
	return &Null{
		BaseHandler: NewBaseHandler(monolog.DEBUG, bubble...),
	}
}

// Handle does nothing.
func (n *Null) Handle(record monolog.Record) error {
	return nil
}

// Close does nothing.
func (n *Null) Close() error {
	return nil
}
