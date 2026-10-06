package handler

import (
	"context"
	"math/rand/v2"

	"github.com/githoober/monogo"
)

// Sampling wraps a handler and samples records based on a 1-in-N sampling factor.
// It is modeled after PHP Monolog's SamplingHandler.
type Sampling struct {
	BaseHandler
	handler     monogo.Handler
	factor      int
	sampler     func() bool
	bypassLevel monogo.Level
	hasBypass   bool
}

var (
	_ monogo.Handler      = (*Sampling)(nil)
	_ monogo.BatchHandler = (*Sampling)(nil)
	_ monogo.Resettable   = (*Sampling)(nil)
)

// NewSampling creates a new Sampling handler wrapping inner with a 1-in-N factor.
// A factor of 1 handles every record (100% sample rate); a factor of 10 handles 1 in 10 records (10%).
// Factors less than 1 are normalized to 1.
func NewSampling(inner monogo.Handler, factor int, opts ...Option) *Sampling {
	if factor < 1 {
		factor = 1
	}

	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	sampler := o.sampler
	if sampler == nil {
		if factor == 1 {
			sampler = func() bool { return true }
		} else {
			sampler = func() bool {
				return rand.IntN(factor) == 0
			}
		}
	}

	s := &Sampling{
		BaseHandler: NewBaseHandler(monogo.DEBUG, opts...),
		handler:     inner,
		factor:      factor,
		sampler:     sampler,
	}

	if o.samplingThreshold != nil {
		s.bypassLevel = *o.samplingThreshold
		s.hasBypass = true
	}

	return s
}

// Factor returns the sampling factor.
func (s *Sampling) Factor() int {
	return s.factor
}

// IsHandling delegates handling check to the wrapped handler.
func (s *Sampling) IsHandling(ctx context.Context, level monogo.Level) bool {
	return s.handler.IsHandling(ctx, level)
}

// shouldSample checks whether a record is selected by sampling or bypasses sampling.
func (s *Sampling) shouldSample(record monogo.Record) bool {
	if s.hasBypass && record.Level >= s.bypassLevel {
		return true
	}
	return s.sampler()
}

// Handle evaluates the sampling decision; if sampled, applies handler processors and forwards to wrapped handler.
func (s *Sampling) Handle(ctx context.Context, record monogo.Record) error {
	if !s.IsHandling(ctx, record.Level) {
		return nil
	}
	if !s.shouldSample(record) {
		return nil
	}
	record = s.ProcessRecord(record)
	return s.handler.Handle(ctx, record)
}

// HandleBatch filters the batch according to sampling rules and forwards surviving records.
func (s *Sampling) HandleBatch(ctx context.Context, records []monogo.Record) error {
	surviving := make([]monogo.Record, 0, len(records))
	for _, rec := range records {
		if s.IsHandling(ctx, rec.Level) && s.shouldSample(rec) {
			surviving = append(surviving, s.ProcessRecord(rec))
		}
	}
	if len(surviving) == 0 {
		return nil
	}

	if bh, ok := s.handler.(monogo.BatchHandler); ok {
		return bh.HandleBatch(ctx, surviving)
	}

	var lastErr error
	for _, rec := range surviving {
		if err := s.handler.Handle(ctx, rec); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Close closes the wrapped handler.
func (s *Sampling) Close(ctx context.Context) error {
	return s.handler.Close(ctx)
}

// Reset resets per-handler processors and resets the wrapped handler if it implements monogo.Resettable.
func (s *Sampling) Reset(ctx context.Context) error {
	var lastErr error
	if err := s.BaseHandler.Reset(ctx); err != nil {
		lastErr = err
	}
	if r, ok := s.handler.(monogo.Resettable); ok {
		if err := r.Reset(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
