package handler

import (
	"context"
	"sync"

	"github.com/githoober/monogo"
)

// FingersCrossed buffers all records until a trigger/action level (e.g. ERROR) is reached.
// Once triggered, it flushes all buffered records to the nested handler and forwards all subsequent records.
type FingersCrossed struct {
	BaseHandler
	handler     monogo.Handler
	actionLevel monogo.Level
	bufferSize  int
	buffer      []monogo.Record
	triggered   bool
	mu          sync.Mutex
}

// NewFingersCrossed creates a FingersCrossed handler with optional configuration options.
// bufferSize specifies the maximum number of records to buffer before triggering (0 = unlimited).
func NewFingersCrossed(handler monogo.Handler, actionLevel monogo.Level, bufferSize int, opts ...Option) *FingersCrossed {
	return &FingersCrossed{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handler:     handler,
		actionLevel: actionLevel,
		bufferSize:  bufferSize,
		buffer:      make([]monogo.Record, 0, bufferSize),
		triggered:   false,
	}
}

// IsHandling returns true for all levels >= handler's minimum level.
func (f *FingersCrossed) IsHandling(level monogo.Level) bool {
	return f.handler.IsHandling(level)
}

// Handle buffers records until actionLevel is met or buffer capacity is exceeded, then flushes and forwards.
func (f *FingersCrossed) Handle(ctx context.Context, record monogo.Record) error {
	record = f.ProcessRecord(record)

	f.mu.Lock()

	// If already triggered, pass straight to nested handler
	if f.triggered {
		f.mu.Unlock()
		return f.handler.Handle(ctx, record)
	}

	// Check if record triggers activation
	if record.Level >= f.actionLevel {
		f.triggered = true
		f.buffer = append(f.buffer, record)
		buffered := f.buffer
		f.buffer = nil
		f.mu.Unlock()

		// Flush all accumulated records to wrapped handler
		if bh, ok := f.handler.(monogo.BatchHandler); ok {
			return bh.HandleBatch(ctx, buffered)
		}
		var lastErr error
		for _, rec := range buffered {
			if err := f.handler.Handle(ctx, rec); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}

	// Buffer the record
	f.buffer = append(f.buffer, record)

	// If buffer limit exceeded, drop oldest record
	if f.bufferSize > 0 && len(f.buffer) > f.bufferSize {
		f.buffer = f.buffer[1:]
	}

	f.mu.Unlock()
	return nil
}

// HandleBatch processes a batch of records.
func (f *FingersCrossed) HandleBatch(ctx context.Context, records []monogo.Record) error {
	for _, rec := range records {
		if err := f.Handle(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// Reset clears buffer and resets triggered status back to un-triggered.
func (f *FingersCrossed) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triggered = false
	f.buffer = make([]monogo.Record, 0, f.bufferSize)
}

// Close flushes buffer if triggered and closes wrapped handler.
func (f *FingersCrossed) Close() error {
	return f.handler.Close()
}
