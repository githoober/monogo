package handler

import (
	"sync"

	"github.com/githoober/monogo"
)

// Buffer buffers records until a capacity limit is reached or flush Level is triggered.
type Buffer struct {
	BaseHandler
	handler     monogo.Handler
	bufferLimit int
	flushLevel  monogo.Level
	buffer      []monogo.Record
	mu          sync.Mutex
}

// NewBuffer creates a Buffer handler with optional configuration options (defaults: bubble=true).
func NewBuffer(handler monogo.Handler, bufferLimit int, flushLevel monogo.Level, opts ...Option) *Buffer {
	return &Buffer{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handler:     handler,
		bufferLimit: bufferLimit,
		flushLevel:  flushLevel,
		buffer:      make([]monogo.Record, 0, bufferLimit),
	}
}

// Handle buffers record and flushes if conditions are met.
func (b *Buffer) Handle(record monogo.Record) error {
	b.mu.Lock()
	b.buffer = append(b.buffer, record)
	shouldFlush := record.Level >= b.flushLevel || (b.bufferLimit > 0 && len(b.buffer) >= b.bufferLimit)
	b.mu.Unlock()

	if shouldFlush {
		return b.Flush()
	}
	return nil
}

// Flush flushes buffered records to wrapped handler.
func (b *Buffer) Flush() error {
	b.mu.Lock()
	records := b.buffer
	b.buffer = make([]monogo.Record, 0, b.bufferLimit)
	b.mu.Unlock()

	var lastErr error
	for _, rec := range records {
		if err := b.handler.Handle(rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Close flushes buffer and closes wrapped handler.
func (b *Buffer) Close() error {
	err := b.Flush()
	if closeErr := b.handler.Close(); closeErr != nil {
		err = closeErr
	}
	return err
}
