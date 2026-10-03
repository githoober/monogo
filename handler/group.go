package handler

import (
	"context"

	"github.com/githoober/monogo"
)

// Group forwards log records to a slice of handlers.
type Group struct {
	BaseHandler
	handlers []monogo.Handler
}

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
func (g *Group) Handle(ctx context.Context, record monogo.Record) error {
	record = g.ProcessRecord(record)
	var lastErr error
	for _, h := range g.handlers {
		if h.IsHandling(ctx, record.Level) {
			if err := h.Handle(ctx, record); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
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
			if err := bh.HandleBatch(ctx, records); err != nil {
				lastErr = err
			}
		} else {
			for _, rec := range records {
				if h.IsHandling(ctx, rec.Level) {
					if err := h.Handle(ctx, rec); err != nil {
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
