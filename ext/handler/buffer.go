package handler

import (
	"context"
	"sync"

	"github.com/githoober/monogo"
)

// Buffer buffers records until a capacity limit is reached or flush Level is triggered.
type Buffer struct {
	BaseHandler
	handler            monogo.Handler
	bufferLimit        int
	flushLevel         monogo.Level
	buffer             []monogo.Record
	lastResetErr       error
	resetErrorCallback func(error)
	mu                 sync.Mutex
}

var _ monogo.Resettable = (*Buffer)(nil)

// NewBuffer creates a Buffer handler with optional configuration options (defaults: bubble=true).
func NewBuffer(handler monogo.Handler, bufferLimit int, flushLevel monogo.Level, opts ...Option) *Buffer {
	base, o := newBaseHandler(monogo.DEBUG, opts...)
	return &Buffer{
		BaseHandler:        base,
		handler:            handler,
		bufferLimit:        bufferLimit,
		flushLevel:         flushLevel,
		buffer:             make([]monogo.Record, 0, bufferLimit),
		resetErrorCallback: o.resetErrorCallback,
	}
}

// IsHandling returns true if the wrapped handler handles the log level.
func (b *Buffer) IsHandling(ctx context.Context, level monogo.Level) bool {
	return b.handler.IsHandling(ctx, level)
}

// Handle buffers record and flushes if conditions are met.
func (b *Buffer) Handle(ctx context.Context, record monogo.Record) error {
	record = b.ProcessRecord(record)

	b.mu.Lock()
	b.buffer = append(b.buffer, record)
	shouldFlush := record.Level >= b.flushLevel || (b.bufferLimit > 0 && len(b.buffer) >= b.bufferLimit)
	b.mu.Unlock()

	if shouldFlush {
		return b.Flush(ctx)
	}
	return nil
}

// Flush flushes buffered records to wrapped handler.
// If the wrapped handler implements monogo.BatchHandler, it calls HandleBatch;
// otherwise, it falls back to calling Handle for each record.
func (b *Buffer) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return nil
	}
	records := b.buffer
	b.buffer = make([]monogo.Record, 0, b.bufferLimit)
	b.mu.Unlock()

	if bh, ok := b.handler.(monogo.BatchHandler); ok {
		return bh.HandleBatch(ctx, records)
	}

	var lastErr error
	for _, rec := range records {
		if err := b.handler.Handle(ctx, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// HandleBatch buffers a batch of records and flushes if conditions are met.
func (b *Buffer) HandleBatch(ctx context.Context, records []monogo.Record) error {
	for _, rec := range records {
		if err := b.Handle(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes buffer and closes wrapped handler.
func (b *Buffer) Close(ctx context.Context) error {
	err := b.Flush(ctx)
	if closeErr := b.handler.Close(ctx); closeErr != nil {
		err = closeErr
	}
	return err
}

// Reset flushes any buffered records to the wrapped handler using the provided context,
// resets per-handler processors, and resets the wrapped handler if it implements monogo.Resettable.
// Any error encountered during Flush or downstream resets is returned directly.
func (b *Buffer) Reset(ctx context.Context) error {
	err := b.Flush(ctx)
	b.mu.Lock()
	b.lastResetErr = err
	cb := b.resetErrorCallback
	b.mu.Unlock()

	if err != nil && cb != nil {
		cb(err)
	}

	if baseErr := b.BaseHandler.Reset(ctx); baseErr != nil && err == nil {
		err = baseErr
	}
	if r, ok := b.handler.(monogo.Resettable); ok {
		if hErr := r.Reset(ctx); hErr != nil && err == nil {
			err = hErr
		}
	}
	return err
}

// LastResetError returns the last error encountered while flushing during Reset(), or nil if none.
func (b *Buffer) LastResetError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastResetErr
}

// Clear discards all buffered records without sending them to the wrapped handler.
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer = make([]monogo.Record, 0, b.bufferLimit)
}
