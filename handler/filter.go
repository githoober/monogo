package handler

import (
	"context"

	"github.com/githoober/monogo"
)

// Filter wraps a handler and filters records based on level or predicate function.
type Filter struct {
	BaseHandler
	handler   monogo.Handler
	minLevel  monogo.Level
	maxLevel  monogo.Level
	predicate func(monogo.Record) bool
}

var _ monogo.Resettable = (*Filter)(nil)

// NewFilter creates a Filter handler for level ranges [minLevel, maxLevel] with optional configuration options.
func NewFilter(handler monogo.Handler, minLevel, maxLevel monogo.Level, opts ...Option) *Filter {
	return &Filter{
		BaseHandler: NewBaseHandler(minLevel, opts...),
		handler:     handler,
		minLevel:    minLevel,
		maxLevel:    maxLevel,
	}
}

// NewFilterFunc creates a Filter handler using custom predicate function with optional configuration options.
func NewFilterFunc(handler monogo.Handler, predicate func(monogo.Record) bool, opts ...Option) *Filter {
	return &Filter{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handler:     handler,
		predicate:   predicate,
	}
}

// IsHandling checks if wrapped handler accepts record and record meets filter condition.
func (f *Filter) IsHandling(ctx context.Context, level monogo.Level) bool {
	if f.predicate == nil {
		if level < f.minLevel || level > f.maxLevel {
			return false
		}
	}
	return f.handler.IsHandling(ctx, level)
}

// Handle routes handling to inner handler if predicate/level check succeeds.
func (f *Filter) Handle(ctx context.Context, record monogo.Record) error {
	if f.predicate != nil {
		if !f.predicate(record) {
			return nil
		}
	} else {
		if record.Level < f.minLevel || record.Level > f.maxLevel {
			return nil
		}
	}

	record = f.ProcessRecord(record)
	return f.handler.Handle(ctx, record)
}

// HandleBatch filters records and forwards matching records to the wrapped handler.
// If the wrapped handler implements monogo.BatchHandler, it calls HandleBatch;
// otherwise, it falls back to calling Handle for each matching record.
func (f *Filter) HandleBatch(ctx context.Context, records []monogo.Record) error {
	filtered := make([]monogo.Record, 0, len(records))
	for _, rec := range records {
		if f.predicate != nil {
			if f.predicate(rec) {
				filtered = append(filtered, f.ProcessRecord(rec))
			}
		} else {
			if rec.Level >= f.minLevel && rec.Level <= f.maxLevel {
				filtered = append(filtered, f.ProcessRecord(rec))
			}
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	if bh, ok := f.handler.(monogo.BatchHandler); ok {
		return bh.HandleBatch(ctx, filtered)
	}

	var lastErr error
	for _, rec := range filtered {
		if err := f.handler.Handle(ctx, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Close closes wrapped handler.
func (f *Filter) Close(ctx context.Context) error {
	return f.handler.Close(ctx)
}

// Reset resets per-handler processors and resets the wrapped handler if it implements monogo.Resettable.
func (f *Filter) Reset(ctx context.Context) error {
	var lastErr error
	if err := f.BaseHandler.Reset(ctx); err != nil {
		lastErr = err
	}
	if r, ok := f.handler.(monogo.Resettable); ok {
		if err := r.Reset(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
