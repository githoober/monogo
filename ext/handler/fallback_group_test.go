package handler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/githoober/monogo"
	corehandler "github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/processor"
)

type fallbackFailingHandler struct {
	err error
}

func (f *fallbackFailingHandler) Handle(ctx context.Context, record monogo.Record) error {
	return f.err
}

func (f *fallbackFailingHandler) HandleBatch(ctx context.Context, records []monogo.Record) error {
	return f.err
}

func (f *fallbackFailingHandler) IsHandling(ctx context.Context, level monogo.Level) bool {
	return true
}

func (f *fallbackFailingHandler) Close(ctx context.Context) error {
	return f.err
}

type fallbackPanickingHandler struct{}

func (p *fallbackPanickingHandler) Handle(ctx context.Context, record monogo.Record) error {
	panic("handler crashed")
}

func (p *fallbackPanickingHandler) HandleBatch(ctx context.Context, records []monogo.Record) error {
	panic("batch crashed")
}

func (p *fallbackPanickingHandler) IsHandling(ctx context.Context, level monogo.Level) bool {
	return true
}

func (p *fallbackPanickingHandler) Close(ctx context.Context) error {
	panic("close crashed")
}

func TestFallbackGroup_PrimarySucceeds(t *testing.T) {
	ctx := t.Context()
	hPrimary := handler.NewTest(monogo.DEBUG)
	hFallback := handler.NewTest(monogo.DEBUG)

	var callbackCalls int
	fg := handler.NewFallbackGroup([]monogo.Handler{hPrimary, hFallback},
		handler.WithFallbackCallback(func(err error, failed monogo.Handler) {
			callbackCalls++
		}),
	)

	rec := monogo.Record{Message: "hello", Level: monogo.INFO}
	if err := fg.Handle(ctx, rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hPrimary.Records()) != 1 {
		t.Errorf("expected primary to receive 1 record, got %d", len(hPrimary.Records()))
	}
	if len(hFallback.Records()) != 0 {
		t.Errorf("expected fallback to receive 0 records, got %d", len(hFallback.Records()))
	}
	if callbackCalls != 0 {
		t.Errorf("expected 0 callback calls, got %d", callbackCalls)
	}
}

func TestFallbackGroup_PrimaryFails_FallbackSucceeds(t *testing.T) {
	ctx := t.Context()
	hPrimary := &fallbackFailingHandler{err: errors.New("network error")}
	hFallback := handler.NewTest(monogo.DEBUG)

	var failedHandler monogo.Handler
	var failedErr error
	fg := handler.NewFallbackGroup([]monogo.Handler{hPrimary, hFallback},
		handler.WithFallbackCallback(func(err error, h monogo.Handler) {
			failedErr = err
			failedHandler = h
		}),
	)

	rec := monogo.Record{Message: "failover test", Level: monogo.ERROR}
	if err := fg.Handle(ctx, rec); err != nil {
		t.Fatalf("unexpected error from fallback: %v", err)
	}

	if failedHandler != hPrimary {
		t.Errorf("expected failedHandler to be primary, got %v", failedHandler)
	}
	if failedErr == nil || failedErr.Error() != "network error" {
		t.Errorf("expected network error, got %v", failedErr)
	}
	if len(hFallback.Records()) != 1 {
		t.Errorf("expected fallback to receive record, got %d", len(hFallback.Records()))
	}
}

func TestFallbackGroup_PrimaryPanics_FallbackSucceeds(t *testing.T) {
	ctx := t.Context()
	hPrimary := &fallbackPanickingHandler{}
	hFallback := handler.NewTest(monogo.DEBUG)

	var callbackCalled bool
	fg := handler.NewFallbackGroup([]monogo.Handler{hPrimary, hFallback},
		handler.WithFallbackCallback(func(err error, h monogo.Handler) {
			callbackCalled = true
		}),
	)

	rec := monogo.Record{Message: "panic test", Level: monogo.WARNING}
	if err := fg.Handle(ctx, rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !callbackCalled {
		t.Errorf("expected fallback callback to be called on panic")
	}
	if len(hFallback.Records()) != 1 {
		t.Errorf("expected fallback to receive 1 record, got %d", len(hFallback.Records()))
	}
}

func TestFallbackGroup_AllFail(t *testing.T) {
	ctx := t.Context()
	h1 := &fallbackFailingHandler{err: errors.New("err 1")}
	h2 := &fallbackFailingHandler{err: errors.New("err 2")}

	fg := handler.NewFallbackGroup([]monogo.Handler{h1, h2})

	rec := monogo.Record{Message: "both fail", Level: monogo.CRITICAL}
	err := fg.Handle(ctx, rec)
	if err == nil {
		t.Fatalf("expected error when all handlers fail")
	}
	if !errors.Is(err, handler.ErrAllFallbacksFailed) {
		t.Errorf("expected ErrAllFallbacksFailed in error chain, got %v", err)
	}
}

func TestFallbackGroup_BatchHandling(t *testing.T) {
	ctx := t.Context()
	hPrimary := &fallbackFailingHandler{err: errors.New("batch failure")}
	hFallback := handler.NewTest(monogo.DEBUG)

	fg := handler.NewFallbackGroup([]monogo.Handler{hPrimary, hFallback})

	records := []monogo.Record{
		{Message: "rec 1", Level: monogo.INFO},
		{Message: "rec 2", Level: monogo.WARNING},
	}

	if err := fg.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected error in HandleBatch: %v", err)
	}

	if len(hFallback.Records()) != 2 {
		t.Errorf("expected fallback to receive 2 records in batch, got %d", len(hFallback.Records()))
	}

	// Empty batch should be a no-op
	if err := fg.HandleBatch(ctx, nil); err != nil {
		t.Fatalf("unexpected error on empty batch: %v", err)
	}
}

func TestFallbackGroup_LifecycleAndInspection(t *testing.T) {
	ctx := t.Context()
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := handler.NewTest(monogo.WARNING)

	fg := handler.NewFallbackGroup([]monogo.Handler{h1, h2},
		handler.WithBubble(false),
		handler.WithProcessor(processor.ProcessId()),
	)

	if fg.Bubble() {
		t.Errorf("expected Bubble() to be false")
	}
	if len(fg.Handlers()) != 2 {
		t.Errorf("expected 2 handlers, got %d", len(fg.Handlers()))
	}
	if !fg.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected IsHandling to be true for DEBUG (handled by h1)")
	}

	// Test Reset
	rec := monogo.Record{Message: "test reset", Level: monogo.INFO}
	_ = fg.Handle(ctx, rec)
	if len(h1.Records()) != 1 {
		t.Fatalf("expected 1 record in h1")
	}

	if err := fg.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from Reset: %v", err)
	}

	// Test Close
	if err := fg.Close(ctx); err != nil {
		t.Fatalf("unexpected error from Close: %v", err)
	}
}

func TestFallbackGroup_NotHandledLevel(t *testing.T) {
	ctx := t.Context()
	hOnlyWarning := corehandler.NewTest(monogo.WARNING)

	fg := handler.NewFallbackGroup([]monogo.Handler{hOnlyWarning})

	// Debug record is not handled
	if fg.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected IsHandling(DEBUG) to be false")
	}

	rec := monogo.Record{Message: "debug message", Level: monogo.DEBUG}
	err := fg.Handle(ctx, rec)
	if !errors.Is(err, monogo.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled, got %v", err)
	}

	errBatch := fg.HandleBatch(ctx, []monogo.Record{rec})
	if !errors.Is(errBatch, monogo.ErrNotHandled) {
		t.Errorf("expected ErrNotHandled for batch, got %v", errBatch)
	}
}
