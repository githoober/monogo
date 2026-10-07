package handler

import (
	"context"
	"errors"

	"github.com/githoober/monogo"
)

// Group forwards log records to a slice of handlers.
type Group struct {
	BaseHandler
	handlers []monogo.Handler
}

var _ monogo.Resettable = (*Group)(nil)

// NewGroup creates a Group handler with optional configuration options.
func NewGroup(handlers []monogo.Handler, opts ...Option) *Group {
	return &Group{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handlers:    handlers,
	}
}

// IsHandling returns true if any nested handler handles the log level.
func (g *Group) IsHandling(ctx context.Context, level monogo.Level) bool {
	for _, h := range g.handlers {
		if h.IsHandling(ctx, level) {
			return true
		}
	}
	return false
}

// Handle sends record to all sub-handlers that handle the record level.
// If all sub-handlers return ErrNotHandled (or no sub-handler handles the record), it returns ErrNotHandled.
// If at least one sub-handler handles the record without operational error, it returns nil.
func (g *Group) Handle(ctx context.Context, record monogo.Record) error {
	record = g.ProcessRecord(record)
	var lastErr error
	handledAny := false
	sawNotHandled := false
	for _, h := range g.handlers {
		if h.IsHandling(ctx, record.Level) {
			if err := h.Handle(ctx, record); err != nil {
				if errors.Is(err, monogo.ErrNotHandled) {
					sawNotHandled = true
				} else {
					lastErr = err
				}
			} else {
				handledAny = true
			}
		}
	}
	if lastErr != nil {
		return lastErr
	}
	if !handledAny && sawNotHandled {
		return monogo.ErrNotHandled
	}
	return nil
}

// HandleBatch forwards a batch of records to all sub-handlers.
// Sub-handlers implementing BatchHandler receive the batch directly;
// others fall back to handling each handled record individually.
func (g *Group) HandleBatch(ctx context.Context, records []monogo.Record) error {
	if len(g.processors) > 0 {
		processed := make([]monogo.Record, len(records))
		for i, rec := range records {
			processed[i] = g.ProcessRecord(rec)
		}
		records = processed
	}

	var lastErr error
	for _, h := range g.handlers {
		if bh, ok := h.(monogo.BatchHandler); ok {
			if err := bh.HandleBatch(ctx, records); err != nil && !errors.Is(err, monogo.ErrNotHandled) {
				lastErr = err
			}
		} else {
			for _, rec := range records {
				if h.IsHandling(ctx, rec.Level) {
					if err := h.Handle(ctx, rec); err != nil && !errors.Is(err, monogo.ErrNotHandled) {
						lastErr = err
					}
				}
			}
		}
	}
	return lastErr
}

// Close closes all nested handlers.
func (g *Group) Close(ctx context.Context) error {
	var lastErr error
	for _, h := range g.handlers {
		if err := h.Close(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Reset resets per-handler processors and all nested handlers implementing monogo.Resettable.
func (g *Group) Reset(ctx context.Context) error {
	var lastErr error
	if err := g.BaseHandler.Reset(ctx); err != nil {
		lastErr = err
	}
	for _, h := range g.handlers {
		if r, ok := h.(monogo.Resettable); ok {
			if err := r.Reset(ctx); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}
