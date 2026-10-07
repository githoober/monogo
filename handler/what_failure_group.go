package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/githoober/monogo"
)

// WhatFailureCallback is called whenever a nested handler encounters an error or panic in WhatFailureGroup.
// This allows observability (logging, telemetry, metrics) without propagating the error up the stack.
type WhatFailureCallback func(err error, h monogo.Handler)

// WithWhatFailureCallback registers an error callback invoked whenever an inner handler fails or panics.
func WithWhatFailureCallback(fn WhatFailureCallback) Option {
	return func(o *options) {
		o.whatFailureCallback = fn
	}
}

// WhatFailureGroup forwards log records to a slice of handlers, safely swallowing and suppressing
// any errors or panics returned by individual sub-handlers during Handle, HandleBatch, or Close.
// This ensures failures in secondary or external logging sinks (e.g., remote services, webhooks, Slack,
// or Elasticsearch) never interrupt primary logging or crash application workflows.
type WhatFailureGroup struct {
	BaseHandler
	handlers []monogo.Handler
	onError  WhatFailureCallback
}

// WhatFailureGroupHandler is an alias for WhatFailureGroup.
type WhatFailureGroupHandler = WhatFailureGroup

// Compile-time interface assertions.
var (
	_ monogo.Handler            = (*WhatFailureGroup)(nil)
	_ monogo.BatchHandler       = (*WhatFailureGroup)(nil)
	_ monogo.Bubbler            = (*WhatFailureGroup)(nil)
	_ monogo.ProcessableHandler = (*WhatFailureGroup)(nil)
	_ monogo.Resettable         = (*WhatFailureGroup)(nil)
)

// NewWhatFailureGroup creates a WhatFailureGroup handler wrapping the given handlers with optional configuration options.
func NewWhatFailureGroup(handlers []monogo.Handler, opts ...Option) *WhatFailureGroup {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	handlersCopy := make([]monogo.Handler, len(handlers))
	copy(handlersCopy, handlers)

	return &WhatFailureGroup{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handlers:    handlersCopy,
		onError:     o.whatFailureCallback,
	}
}

// Handlers returns a copy of the nested handlers.
func (w *WhatFailureGroup) Handlers() []monogo.Handler {
	cp := make([]monogo.Handler, len(w.handlers))
	copy(cp, w.handlers)
	return cp
}

// IsHandling returns true if any nested handler handles the log level.
// Any panic in a nested handler's IsHandling is suppressed and forwarded to onError.
func (w *WhatFailureGroup) IsHandling(ctx context.Context, level monogo.Level) bool {
	for _, h := range w.handlers {
		if safeIsHandling(h, w.onError, ctx, level) {
			return true
		}
	}
	return false
}

// Handle sends record to all sub-handlers that handle the record level.
// Any operational error or panic returned by a sub-handler is suppressed and forwarded to the optional callback.
// If all sub-handlers return ErrNotHandled, it returns ErrNotHandled so bubbling can continue.
// If at least one sub-handler handled the record (or failed operationally), it returns nil.
func (w *WhatFailureGroup) Handle(ctx context.Context, record monogo.Record) error {
	record = w.ProcessRecord(record)
	handledAny := false
	sawNotHandled := false
	for _, h := range w.handlers {
		if safeIsHandling(h, w.onError, ctx, record.Level) {
			err := invokeSafe(h, w.onError, func() error {
				return h.Handle(ctx, record)
			})
			if errors.Is(err, monogo.ErrNotHandled) {
				sawNotHandled = true
			} else {
				handledAny = true
			}
		}
	}
	if !handledAny && sawNotHandled {
		return monogo.ErrNotHandled
	}
	return nil
}

// HandleBatch forwards a batch of records to all sub-handlers.
// Sub-handlers implementing BatchHandler receive the batch directly;
// others fall back to handling each handled record individually.
// Any errors or panics returned by sub-handlers are suppressed and forwarded to the optional callback.
// Always returns nil.
func (w *WhatFailureGroup) HandleBatch(ctx context.Context, records []monogo.Record) error {
	if len(w.processors) > 0 {
		processed := make([]monogo.Record, len(records))
		for i, rec := range records {
			processed[i] = w.ProcessRecord(rec)
		}
		records = processed
	}

	for _, h := range w.handlers {
		if bh, ok := h.(monogo.BatchHandler); ok {
			_ = invokeSafe(h, w.onError, func() error {
				return bh.HandleBatch(ctx, records)
			})
		} else {
			for _, rec := range records {
				if safeIsHandling(h, w.onError, ctx, rec.Level) {
					_ = invokeSafe(h, w.onError, func() error {
						return h.Handle(ctx, rec)
					})
				}
			}
		}
	}
	return nil
}

// Close closes all nested handlers. Any error or panic returned by a sub-handler is suppressed
// and forwarded to the optional callback. Always returns nil.
func (w *WhatFailureGroup) Close(ctx context.Context) error {
	for _, h := range w.handlers {
		_ = invokeSafe(h, w.onError, func() error {
			return h.Close(ctx)
		})
	}
	return nil
}

// Reset resets per-handler processors and all nested handlers implementing monogo.Resettable,
// safely suppressing and reporting any panics via the error callback. Always returns nil.
func (w *WhatFailureGroup) Reset(ctx context.Context) error {
	_ = w.BaseHandler.Reset(ctx)
	for _, h := range w.handlers {
		if r, ok := h.(monogo.Resettable); ok {
			_ = invokeSafe(h, w.onError, func() error {
				return r.Reset(ctx)
			})
		}
	}
	return nil
}

func safeIsHandling(h monogo.Handler, onError WhatFailureCallback, ctx context.Context, level monogo.Level) bool {
	var handled bool
	_ = invokeSafe(h, onError, func() error {
		handled = h.IsHandling(ctx, level)
		return nil
	})
	return handled
}

func invokeSafe(h monogo.Handler, onError WhatFailureCallback, fn func() error) error {
	var err error
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic in handler: %v", r)
			}
			if onError != nil {
				func() {
					defer func() { _ = recover() }()
					onError(err, h)
				}()
			}
		}
	}()

	err = fn()
	if err != nil && !errors.Is(err, monogo.ErrNotHandled) {
		if onError != nil {
			func() {
				defer func() { _ = recover() }()
				onError(err, h)
			}()
		}
	}
	return err
}
