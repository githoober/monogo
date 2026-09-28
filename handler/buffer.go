package handler

import (
	"context"
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
	if ctx == nil {
		ctx = context.Background()
	}

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
func (b *Buffer) Close() error {
	err := b.Flush(context.Background())
	if closeErr := b.handler.Close(); closeErr != nil {
		err = closeErr
	}
	return err
}
