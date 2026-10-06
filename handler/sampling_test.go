package handler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func TestSamplingHandler_Factor1(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	sh := handler.NewSampling(testH, 1)

	if sh.Factor() != 1 {
		t.Errorf("expected factor 1, got %d", sh.Factor())
	}

	for i := 0; i < 10; i++ {
		if err := sh.Handle(ctx, monogo.Record{Message: "msg", Level: monogo.INFO}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	records := testH.Records()
	if len(records) != 10 {
		t.Errorf("expected 10 records, got %d", len(records))
	}
}

func TestSamplingHandler_NegativeOrZeroFactor(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	sh0 := handler.NewSampling(testH, 0)
	if sh0.Factor() != 1 {
		t.Errorf("expected normalized factor 1, got %d", sh0.Factor())
	}
	shNeg := handler.NewSampling(testH, -5)
	if shNeg.Factor() != 1 {
		t.Errorf("expected normalized factor 1, got %d", shNeg.Factor())
	}
}

func TestSamplingHandler_CustomSampler(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)

	// Custom sampler that accepts every other record (alternating)
	var state bool
	sh := handler.NewSampling(testH, 2, handler.WithSampler(func() bool {
		state = !state
		return state
	}))

	for i := 0; i < 10; i++ {
		if err := sh.Handle(ctx, monogo.Record{Message: "msg", Level: monogo.INFO}); err != nil && !errors.Is(err, monogo.ErrNotHandled) {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Should accept 5 out of 10
	records := testH.Records()
	if len(records) != 5 {
		t.Errorf("expected 5 records, got %d", len(records))
	}
}

func TestSamplingHandler_ThresholdBypass(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)

	// Sampler that rejects all sampled logs, but has threshold ERROR
	sh := handler.NewSampling(testH, 10,
		handler.WithSampler(func() bool { return false }),
		handler.WithSamplingThreshold(monogo.ERROR),
	)

	// DEBUG and INFO should be dropped by sampler (returning ErrNotHandled)
	if err := sh.Handle(ctx, monogo.Record{Message: "debug", Level: monogo.DEBUG}); !errors.Is(err, monogo.ErrNotHandled) {
		t.Fatalf("expected ErrNotHandled for dropped debug record, got: %v", err)
	}
	if err := sh.Handle(ctx, monogo.Record{Message: "info", Level: monogo.INFO}); !errors.Is(err, monogo.ErrNotHandled) {
		t.Fatalf("expected ErrNotHandled for dropped info record, got: %v", err)
	}
	// ERROR and CRITICAL should bypass sampler and be emitted
	if err := sh.Handle(ctx, monogo.Record{Message: "err", Level: monogo.ERROR}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sh.Handle(ctx, monogo.Record{Message: "crit", Level: monogo.CRITICAL}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := testH.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Message != "err" || records[1].Message != "crit" {
		t.Errorf("unexpected records: %v", records)
	}
}

func TestSamplingHandler_HandleBatch(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)

	// Sampler accepts only odd indexes (1st, 3rd)
	count := 0
	sh := handler.NewSampling(testH, 2, handler.WithSampler(func() bool {
		count++
		return count%2 != 0
	}))

	records := []monogo.Record{
		{Message: "1", Level: monogo.INFO},
		{Message: "2", Level: monogo.INFO},
		{Message: "3", Level: monogo.INFO},
		{Message: "4", Level: monogo.INFO},
	}

	if err := sh.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	res := testH.Records()
	if len(res) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res))
	}
	if res[0].Message != "1" || res[1].Message != "3" {
		t.Errorf("unexpected batch records: %v", res)
	}

	// Empty batch or all rejected
	allReject := handler.NewSampling(testH, 10, handler.WithSampler(func() bool { return false }))
	if err := allReject.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSamplingHandler_HandleBatchNonBatchFallback(t *testing.T) {
	ctx := t.Context()
	nonBatch := &customNonBatchHandler{minLevel: monogo.DEBUG}
	sh := handler.NewSampling(nonBatch, 1)

	records := []monogo.Record{
		{Message: "a", Level: monogo.INFO},
		{Message: "b", Level: monogo.INFO},
	}

	if err := sh.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nonBatch.handled) != 2 {
		t.Errorf("expected 2 records handled individually, got %d", len(nonBatch.handled))
	}
}

type customNonBatchHandler struct {
	minLevel monogo.Level
	handled  []monogo.Record
}

func (c *customNonBatchHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	return level >= c.minLevel
}

func (c *customNonBatchHandler) Handle(_ context.Context, r monogo.Record) error {
	c.handled = append(c.handled, r)
	return nil
}

func (c *customNonBatchHandler) Close(_ context.Context) error {
	return nil
}

func TestSamplingHandler_LifecycleAndReset(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	sh := handler.NewSampling(testH, 1,
		handler.WithProcessor(processor.Tag("sampled", "true")),
		handler.WithBubble(false),
	)

	if !sh.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected IsHandling to be true")
	}
	if sh.Bubble() {
		t.Errorf("expected Bubble to be false")
	}

	if err := sh.Handle(ctx, monogo.Record{Message: "test", Level: monogo.INFO}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Extra["sampled"] != "true" {
		t.Errorf("expected processor tag sampled=true, got %v", records[0].Extra["sampled"])
	}

	// Reset
	if err := sh.Reset(ctx); err != nil {
		t.Fatalf("unexpected error during reset: %v", err)
	}
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH records to be cleared after reset")
	}

	// Close
	if err := sh.Close(ctx); err != nil {
		t.Fatalf("unexpected error during close: %v", err)
	}
}

func TestSamplingHandler_DefaultRandomExecution(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	// Factor 2 with default math/rand/v2 sampler
	sh := handler.NewSampling(testH, 2)

	for i := 0; i < 100; i++ {
		_ = sh.Handle(ctx, monogo.Record{Message: "random", Level: monogo.INFO})
	}

	records := testH.Records()
	// Out of 100 with 50% probability, we expect between 10 and 90 records
	if len(records) == 0 || len(records) == 100 {
		t.Errorf("expected reasonable sampling count, got %d out of 100", len(records))
	}
}

func TestSamplingHandler_Errors(t *testing.T) {
	ctx := t.Context()
	failH := &samplingFailingHandler{err: errors.New("write error")}
	sh := handler.NewSampling(failH, 1)

	if err := sh.Handle(ctx, monogo.Record{Message: "fail", Level: monogo.INFO}); err == nil {
		t.Errorf("expected error from failing inner handler")
	}
	if err := sh.HandleBatch(ctx, []monogo.Record{{Message: "fail", Level: monogo.INFO}}); err == nil {
		t.Errorf("expected error from failing inner handler batch")
	}
}

type samplingFailingHandler struct {
	err error
}

func (f *samplingFailingHandler) IsHandling(_ context.Context, _ monogo.Level) bool { return true }
func (f *samplingFailingHandler) Handle(_ context.Context, _ monogo.Record) error    { return f.err }
func (f *samplingFailingHandler) Close(_ context.Context) error                     { return f.err }

func TestSamplingHandler_Bubbling(t *testing.T) {
	ctx := t.Context()
	sampledH := handler.NewTest(monogo.DEBUG)
	fallbackH := handler.NewTest(monogo.DEBUG)

	// Alternate acceptance: 5 accepted, 5 rejected
	var state bool
	sh := handler.NewSampling(sampledH, 2,
		handler.WithSampler(func() bool {
			state = !state
			return state
		}),
		handler.WithBubble(false),
	)

	logger := monogo.New("app", []monogo.Handler{sh, fallbackH}, nil)

	for i := 0; i < 10; i++ {
		if err := logger.Info(ctx, "msg"); err != nil {
			t.Fatalf("unexpected logger error: %v", err)
		}
	}

	// 5 records sampled into sampledH (and stopped from bubbling because Bubble=false)
	if len(sampledH.Records()) != 5 {
		t.Errorf("expected 5 records in sampledH, got %d", len(sampledH.Records()))
	}
	// The other 5 records were rejected by sampling and bubbled down to fallbackH!
	if len(fallbackH.Records()) != 5 {
		t.Errorf("expected 5 rejected records to bubble to fallbackH, got %d", len(fallbackH.Records()))
	}
}
