package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/githoober/monogo"
)

// ErrAllFallbacksFailed is returned when every child handler in FallbackGroup fails to handle the record.
var ErrAllFallbacksFailed = errors.New("all fallback handlers failed")

// FallbackGroup forwards log records to child handlers sequentially until one successfully handles the record.
// If a child handler fails or panics, the failure is caught, an optional callback is invoked, and the record
// is forwarded to the next handler in priority order.
// Modeled after PHP Monolog's FallbackGroupHandler.
type FallbackGroup struct {
	BaseHandler
	handlers   []monogo.Handler
	onFallback FallbackCallback
}

// FallbackGroupHandler is an alias for FallbackGroup.
type FallbackGroupHandler = FallbackGroup

// Compile-time interface assertions.
var (
	_ monogo.Handler            = (*FallbackGroup)(nil)
	_ monogo.BatchHandler       = (*FallbackGroup)(nil)
	_ monogo.Bubbler            = (*FallbackGroup)(nil)
	_ monogo.ProcessableHandler = (*FallbackGroup)(nil)
	_ monogo.Resettable         = (*FallbackGroup)(nil)
)

// NewFallbackGroup creates a FallbackGroup handler wrapping the provided handlers in priority order.
func NewFallbackGroup(handlers []monogo.Handler, opts ...Option) *FallbackGroup {
	base, o := newBaseHandler(monogo.DEBUG, opts...)
	handlersCopy := make([]monogo.Handler, len(handlers))
	copy(handlersCopy, handlers)

	return &FallbackGroup{
		BaseHandler: base,
		handlers:    handlersCopy,
		onFallback:  o.fallbackCallback,
	}
}

// Handlers returns a copy of the nested child handlers.
func (f *FallbackGroup) Handlers() []monogo.Handler {
	cp := make([]monogo.Handler, len(f.handlers))
	copy(cp, f.handlers)
	return cp
}

// IsHandling returns true if any nested handler handles the log level.
func (f *FallbackGroup) IsHandling(ctx context.Context, level monogo.Level) bool {
	for _, h := range f.handlers {
		if safeIsHandling(h, func(err error, h monogo.Handler) {
			if f.onFallback != nil {
				f.onFallback(err, h)
			}
		}, ctx, level) {
			return true
		}
	}
	return false
}

// Handle forwards the record to child handlers in priority order until one succeeds.
func (f *FallbackGroup) Handle(ctx context.Context, record monogo.Record) error {
	record = f.ProcessRecord(record)
	var errs []error
	handledAny := false

	for _, h := range f.handlers {
		if !safeIsHandling(h, func(err error, h monogo.Handler) {
			if f.onFallback != nil {
				f.onFallback(err, h)
			}
		}, ctx, record.Level) {
			continue
		}

		handledAny = true
		cloned := record.Clone()
		err := invokeSafe(h, func(err error, h monogo.Handler) {
			if f.onFallback != nil {
				f.onFallback(err, h)
			}
		}, func() error {
			return h.Handle(ctx, cloned)
		})

		if err == nil {
			return nil // Success! One handler handled it, stop here.
		}

		if !errors.Is(err, monogo.ErrNotHandled) {
			errs = append(errs, fmt.Errorf("%T: %w", h, err))
		}
	}

	if !handledAny {
		return monogo.ErrNotHandled
	}

	if len(errs) > 0 {
		return errors.Join(append([]error{ErrAllFallbacksFailed}, errs...)...)
	}
	return nil
}

// HandleBatch forwards records to child handlers in priority order until one succeeds.
func (f *FallbackGroup) HandleBatch(ctx context.Context, records []monogo.Record) error {
	if len(records) == 0 {
		return nil
	}

	if len(f.Processors()) > 0 {
		processed := make([]monogo.Record, len(records))
		for i, r := range records {
			processed[i] = f.ProcessRecord(r)
		}
		records = processed
	}

	var errs []error
	handledAny := false

	for _, h := range f.handlers {
		var applicable []monogo.Record
		for _, r := range records {
			if safeIsHandling(h, func(err error, h monogo.Handler) {
				if f.onFallback != nil {
					f.onFallback(err, h)
				}
			}, ctx, r.Level) {
				applicable = append(applicable, r.Clone())
			}
		}

		if len(applicable) == 0 {
			continue
		}

		handledAny = true
		err := invokeSafe(h, func(err error, h monogo.Handler) {
			if f.onFallback != nil {
				f.onFallback(err, h)
			}
		}, func() error {
			if bh, ok := h.(monogo.BatchHandler); ok {
				return bh.HandleBatch(ctx, applicable)
			}
			for _, rec := range applicable {
				if hErr := h.Handle(ctx, rec); hErr != nil {
					return hErr
				}
			}
			return nil
		})

		if err == nil {
			return nil // One handler successfully handled the batch, stop here.
		}

		if !errors.Is(err, monogo.ErrNotHandled) {
			errs = append(errs, fmt.Errorf("%T: %w", h, err))
		}
	}

	if !handledAny {
		return monogo.ErrNotHandled
	}

	if len(errs) > 0 {
		return errors.Join(append([]error{ErrAllFallbacksFailed}, errs...)...)
	}
	return nil
}

// Close closes all child handlers.
func (f *FallbackGroup) Close(ctx context.Context) error {
	var errs []error
	for _, h := range f.handlers {
		if err := invokeSafe(h, func(err error, h monogo.Handler) {
			if f.onFallback != nil {
				f.onFallback(err, h)
			}
		}, func() error {
			return h.Close(ctx)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Reset resets per-handler processors and all resettable child handlers.
func (f *FallbackGroup) Reset(ctx context.Context) error {
	var errs []error
	if err := f.BaseHandler.Reset(ctx); err != nil {
		errs = append(errs, err)
	}
	for _, h := range f.handlers {
		if r, ok := h.(monogo.Resettable); ok {
			if err := invokeSafe(h, func(err error, h monogo.Handler) {
				if f.onFallback != nil {
					f.onFallback(err, h)
				}
			}, func() error {
				return r.Reset(ctx)
			}); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
