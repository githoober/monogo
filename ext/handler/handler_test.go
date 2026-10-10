package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/ext/processor"
	"github.com/githoober/monogo/formatter"
	corehandler "github.com/githoober/monogo/handler"
)

var (
	_ monogo.ProcessableHandler = (*handler.Buffer)(nil)
	_ monogo.ProcessableHandler = (*handler.Filter)(nil)
	_ monogo.ProcessableHandler = (*handler.Group)(nil)
	_ monogo.ProcessableHandler = (*handler.Deduplication)(nil)
	_ monogo.ProcessableHandler = (*handler.WhatFailureGroup)(nil)
)


type batchTrackingHandler struct {
	handledRecords []monogo.Record
	batchCalls     int
	handleCalls    int
	minLevel       monogo.Level
}

func newBatchTrackingHandler(minLevel ...monogo.Level) *batchTrackingHandler {
	lvl := monogo.DEBUG
	if len(minLevel) > 0 {
		lvl = minLevel[0]
	}
	return &batchTrackingHandler{minLevel: lvl}
}

func (b *batchTrackingHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	return level >= b.minLevel
}

func (b *batchTrackingHandler) Handle(_ context.Context, record monogo.Record) error {
	b.handleCalls++
	b.handledRecords = append(b.handledRecords, record)
	return nil
}

func (b *batchTrackingHandler) HandleBatch(ctx context.Context, records []monogo.Record) error {
	b.batchCalls++
	b.handledRecords = append(b.handledRecords, records...)
	return nil
}

func (b *batchTrackingHandler) Close(ctx context.Context) error {
	return nil
}

func (b *batchTrackingHandler) Records() []monogo.Record {
	return b.handledRecords
}


type mockResettableProc struct {
	name       string
	resetCount int
}

func (m *mockResettableProc) Process(r monogo.Record) monogo.Record {
	return r
}

func (m *mockResettableProc) Reset(_ context.Context) error {
	m.resetCount++
	return nil
}


type nonBatchMockHandler struct {
	records     []monogo.Record
	handleCalls int
	minLevel    monogo.Level
}

func (n *nonBatchMockHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	return level >= n.minLevel
}

func (n *nonBatchMockHandler) Handle(ctx context.Context, record monogo.Record) error {
	n.handleCalls++
	n.records = append(n.records, record)
	return nil
}

func (n *nonBatchMockHandler) Close(ctx context.Context) error {
	return nil
}

type mockDedupStore struct {
	calledIsDuplicate bool
	calledReset       bool
}

func (m *mockDedupStore) IsDuplicate(key string, now time.Time, window time.Duration) bool {
	m.calledIsDuplicate = true
	return false
}

func (m *mockDedupStore) Reset() {
	m.calledReset = true
}

type failingMockHandler struct {
	name             string
	minLevel         monogo.Level
	panicIsHandling  bool
	failHandle       bool
	panicHandle      bool
	panicHandleError bool
	failHandleBatch  bool
	panicHandleBatch bool
	failClose        bool
	panicClose       bool
	handleCalls      int
	batchCalls       int
	closeCalls       int
	records          []monogo.Record
}

func (f *failingMockHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	if f.panicIsHandling {
		panic("isHandling panic simulated: " + f.name)
	}
	return level >= f.minLevel
}

func (f *failingMockHandler) Handle(_ context.Context, record monogo.Record) error {
	f.handleCalls++
	if f.panicHandleError {
		panic(errors.New("handle panic error object: " + f.name))
	}
	if f.panicHandle {
		panic("handle panic simulated: " + f.name)
	}
	if f.failHandle {
		return fmt.Errorf("handle error simulated: %s", f.name)
	}
	f.records = append(f.records, record)
	return nil
}

func (f *failingMockHandler) HandleBatch(_ context.Context, records []monogo.Record) error {
	f.batchCalls++
	if f.panicHandleBatch {
		panic("handleBatch panic simulated: " + f.name)
	}
	if f.failHandleBatch {
		return fmt.Errorf("handleBatch error simulated: %s", f.name)
	}
	f.records = append(f.records, records...)
	return nil
}

func (f *failingMockHandler) Close(_ context.Context) error {
	f.closeCalls++
	if f.panicClose {
		panic("close panic simulated: " + f.name)
	}
	if f.failClose {
		return fmt.Errorf("close error simulated: %s", f.name)
	}
	return nil
}

type failingNonBatchHandler struct {
	name            string
	minLevel        monogo.Level
	panicIsHandling bool
	failHandle      bool
	panicHandle     bool
	handleCalls     int
	records         []monogo.Record
}

func (f *failingNonBatchHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	if f.panicIsHandling {
		panic("non-batch isHandling panic simulated: " + f.name)
	}
	return level >= f.minLevel
}

func (f *failingNonBatchHandler) Handle(_ context.Context, record monogo.Record) error {
	f.handleCalls++
	if f.panicHandle {
		panic("non-batch handle panic simulated: " + f.name)
	}
	if f.failHandle {
		return fmt.Errorf("non-batch handle error simulated: %s", f.name)
	}
	f.records = append(f.records, record)
	return nil
}

func (f *failingNonBatchHandler) Close(_ context.Context) error {
	return nil
}

type mockPanicResetHandler struct {
	monogo.Handler
	resetCount int
}

func (m *mockPanicResetHandler) Reset(_ context.Context) error {
	m.resetCount++
	panic("simulated reset panic in handler")
}

type failingHandler struct {
	err error
}

func (f *failingHandler) IsHandling(ctx context.Context, level monogo.Level) bool {
	return true
}

func (f *failingHandler) Handle(ctx context.Context, record monogo.Record) error {
	return f.err
}

func (f *failingHandler) Close(ctx context.Context) error {
	return nil
}

func TestRotatingFileHandler(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotating.log")

	rotH := handler.NewRotatingFile(logPath, monogo.INFO,
		handler.WithMaxSize(1),
		handler.WithMaxBackups(2),
		handler.WithMaxAge(7),
	)
	defer func() { _ = rotH.Close(context.Background()) }()

	if !rotH.Bubble() {
		t.Errorf("expected default Bubble to be true")
	}

	logger := monogo.New("rot-app", []monogo.Handler{rotH}, nil)

	err := logger.Info(context.Background(), "rotating file log message", map[string]interface{}{"test": "rotation"})
	if err != nil {
		t.Fatalf("unexpected error logging to rotating file: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read rotated log file: %v", err)
	}

	if !strings.Contains(string(content), "rot-app.INFO: rotating file log message") {
		t.Errorf("rotated log file missing expected content, got: %s", string(content))
	}
}

func TestRotatingFileHandlerBubbling(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotating_bubble.log")

	rotH := handler.NewRotatingFile(logPath, monogo.ERROR,
		handler.WithBubble(false),
		handler.WithRotation(handler.RotatingFileOptions{
			MaxSizeMB:  1,
			MaxBackups: 2,
		}),
	)
	defer func() { _ = rotH.Close(context.Background()) }()

	if rotH.Bubble() {
		t.Errorf("expected Bubble to be false with WithBubble(false)")
	}
}

func TestFilterHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	filterH := handler.NewFilter(testH, monogo.INFO, monogo.ERROR)

	_ = filterH.Handle(context.Background(), monogo.Record{Message: "debug", Level: monogo.DEBUG})
	_ = filterH.Handle(context.Background(), monogo.Record{Message: "info", Level: monogo.INFO})
	_ = filterH.Handle(context.Background(), monogo.Record{Message: "crit", Level: monogo.CRITICAL})

	recs := testH.Records()
	if len(recs) != 1 || recs[0].Message != "info" {
		t.Errorf("Filter handler failed, expected only 'info', got: %v", recs)
	}
}

func TestFilterHandler_Bubbling(t *testing.T) {
	ctx := t.Context()
	inner := handler.NewTest(monogo.DEBUG)
	fallback := handler.NewTest(monogo.DEBUG)
	filterH := handler.NewFilter(inner, monogo.INFO, monogo.WARNING, handler.WithBubble(false))

	if filterH.Bubble() {
		t.Fatalf("expected Filter.Bubble() to be false")
	}

	logger := monogo.New("app", []monogo.Handler{filterH, fallback}, nil)

	_ = logger.Debug(ctx, "debug msg") // Filter rejects -> bubbles to fallback
	_ = logger.Info(ctx, "info msg")   // Filter handles -> stops bubbling
	_ = logger.Error(ctx, "error msg") // Filter rejects -> bubbles to fallback

	if len(inner.Records()) != 1 || inner.Records()[0].Message != "info msg" {
		t.Errorf("inner handler expected 1 record (info msg), got: %v", inner.Records())
	}
	if len(fallback.Records()) != 2 {
		t.Errorf("fallback handler expected 2 records (debug and error), got: %d", len(fallback.Records()))
	}
}

func TestGroupHandler(t *testing.T) {
	t1 := handler.NewTest(monogo.DEBUG)
	t2 := handler.NewTest(monogo.WARNING)
	group := handler.NewGroup([]monogo.Handler{t1, t2})

	_ = group.Handle(context.Background(), monogo.Record{Message: "info msg", Level: monogo.INFO})
	_ = group.Handle(context.Background(), monogo.Record{Message: "warn msg", Level: monogo.WARNING})

	if len(t1.Records()) != 2 {
		t.Errorf("t1 should have 2 records, got %d", len(t1.Records()))
	}
	if len(t2.Records()) != 1 {
		t.Errorf("t2 should have 1 record, got %d", len(t2.Records()))
	}
}

func TestGroupHandler_NotHandledComposition(t *testing.T) {
	ctx := t.Context()

	// Case 1: One child handles, one child returns ErrNotHandled.
	// Group successfully handles the record and suppresses bubbling to outer fallback.
	{
		hPrimary := handler.NewTest(monogo.DEBUG)
		hSampledInner := handler.NewTest(monogo.DEBUG)
		// Sampler always rejects (factor 100, sampler returns false)
		hSampling := handler.NewSampling(hSampledInner, 100,
			handler.WithSampler(func() bool { return false }),
		)

		group := handler.NewGroup([]monogo.Handler{hPrimary, hSampling}, handler.WithBubble(false))
		outerFallback := handler.NewTest(monogo.DEBUG)

		logger := monogo.New("app", []monogo.Handler{group, outerFallback}, nil)
		if err := logger.Info(ctx, "mixed group record"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(hPrimary.Records()) != 1 {
			t.Errorf("expected hPrimary to receive 1 record, got %d", len(hPrimary.Records()))
		}
		if len(outerFallback.Records()) != 0 {
			t.Errorf("expected outerFallback to receive 0 records (bubbling stopped), got %d", len(outerFallback.Records()))
		}
	}

	// Case 2: All children return ErrNotHandled.
	// Group propagates ErrNotHandled, allowing bubbling to outer fallback.
	{
		hSampledInner1 := handler.NewTest(monogo.DEBUG)
		hSampledInner2 := handler.NewTest(monogo.DEBUG)
		hSampling1 := handler.NewSampling(hSampledInner1, 100, handler.WithSampler(func() bool { return false }))
		hSampling2 := handler.NewSampling(hSampledInner2, 100, handler.WithSampler(func() bool { return false }))

		group := handler.NewGroup([]monogo.Handler{hSampling1, hSampling2}, handler.WithBubble(false))
		outerFallback := handler.NewTest(monogo.DEBUG)

		logger := monogo.New("app", []monogo.Handler{group, outerFallback}, nil)
		if err := logger.Info(ctx, "unhandled group record"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(outerFallback.Records()) != 1 {
			t.Errorf("expected outerFallback to receive 1 record because group did not handle, got %d", len(outerFallback.Records()))
		}
	}
}

func TestBufferHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	bufH := handler.NewBuffer(testH, 3, monogo.ERROR)

	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 1", Level: monogo.INFO})
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 2", Level: monogo.INFO})

	if len(testH.Records()) != 0 {
		t.Errorf("buffer should not have flushed yet")
	}

	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 3 error", Level: monogo.ERROR})
	if len(testH.Records()) != 3 {
		t.Errorf("buffer should have flushed 3 records, got %d", len(testH.Records()))
	}
}

func TestBufferHandlerFlushesViaHandleBatch(t *testing.T) {
	inner := newBatchTrackingHandler(monogo.DEBUG)
	bufH := handler.NewBuffer(inner, 10, monogo.ERROR)

	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 1", Level: monogo.DEBUG})
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 2", Level: monogo.INFO})

	if inner.batchCalls != 0 {
		t.Fatalf("expected 0 batch calls before flush, got %d", inner.batchCalls)
	}

	// Trigger flush via ERROR record
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 3", Level: monogo.ERROR})

	if inner.batchCalls != 1 {
		t.Errorf("expected exactly 1 HandleBatch call on flush, got %d", inner.batchCalls)
	}
	if len(inner.Records()) != 3 {
		t.Errorf("expected 3 records in wrapped handler, got %d", len(inner.Records()))
	}
}

func TestFilterHandlerHandleBatch(t *testing.T) {
	inner := newBatchTrackingHandler(monogo.DEBUG)
	filter := handler.NewFilter(inner, monogo.INFO, monogo.WARNING)

	records := []monogo.Record{
		{Message: "debug", Level: monogo.DEBUG},  // filtered out
		{Message: "info", Level: monogo.INFO},    // passes
		{Message: "warn", Level: monogo.WARNING}, // passes
		{Message: "error", Level: monogo.ERROR},  // filtered out
	}

	if err := filter.HandleBatch(context.Background(), records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inner.batchCalls != 1 {
		t.Errorf("expected 1 HandleBatch call, got %d", inner.batchCalls)
	}
	recs := inner.Records()
	if len(recs) != 2 {
		t.Fatalf("expected 2 filtered records, got %d", len(recs))
	}
	if recs[0].Message != "info" || recs[1].Message != "warn" {
		t.Errorf("unexpected records: %v", recs)
	}
}

func TestGroupHandlerHandleBatch(t *testing.T) {
	h1 := newBatchTrackingHandler(monogo.DEBUG)
	h2 := newBatchTrackingHandler(monogo.DEBUG)
	group := handler.NewGroup([]monogo.Handler{h1, h2})

	records := []monogo.Record{
		{Message: "group 1", Level: monogo.INFO},
		{Message: "group 2", Level: monogo.WARNING},
	}

	if err := group.HandleBatch(context.Background(), records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1.batchCalls != 1 || h2.batchCalls != 1 {
		t.Errorf("expected both sub-handlers to receive 1 HandleBatch call, got h1=%d, h2=%d", h1.batchCalls, h2.batchCalls)
	}
	if len(h1.Records()) != 2 || len(h2.Records()) != 2 {
		t.Errorf("expected both sub-handlers to have 2 records, got h1=%d, h2=%d", len(h1.Records()), len(h2.Records()))
	}
}

func TestBufferFallbackForNonBatchHandler(t *testing.T) {
	inner := &nonBatchMockHandler{minLevel: monogo.DEBUG}
	bufH := handler.NewBuffer(inner, 3, monogo.ERROR)

	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 1", Level: monogo.INFO})
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 2", Level: monogo.INFO})
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 3", Level: monogo.INFO}) // flushes due to limit 3

	if inner.handleCalls != 3 {
		t.Errorf("expected 3 fallback Handle calls, got %d", inner.handleCalls)
	}
	if len(inner.records) != 3 {
		t.Errorf("expected 3 records, got %d", len(inner.records))
	}
}

func TestBufferHandlerWithProcessor(t *testing.T) {
	inner := handler.NewTest(monogo.DEBUG)
	bufH := handler.NewBuffer(inner, 2, monogo.ERROR,
		handler.WithProcessor(processor.Tag("buffer_tag", "buffered_val")),
	)

	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 1", Level: monogo.INFO})
	_ = bufH.Handle(context.Background(), monogo.Record{Message: "msg 2", Level: monogo.INFO}) // flushes due to limit 2

	recs := inner.Records()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records flushed to inner handler, got %d", len(recs))
	}
	for i, r := range recs {
		if r.Extra["buffer_tag"] != "buffered_val" {
			t.Errorf("record %d missing buffer_tag, got: %v", i, r.Extra["buffer_tag"])
		}
	}
}

func TestFilterHandlerWithProcessor(t *testing.T) {
	inner := handler.NewTest(monogo.DEBUG)
	filterH := handler.NewFilter(inner, monogo.WARNING, monogo.CRITICAL,
		handler.WithProcessor(processor.Tag("filter_applied", true)),
	)

	// Below minLevel -> discarded, processor not run
	_ = filterH.Handle(context.Background(), monogo.Record{Message: "info message", Level: monogo.INFO})
	if len(inner.Records()) != 0 {
		t.Errorf("expected 0 records, got %d", len(inner.Records()))
	}

	// Within range -> processed and forwarded
	_ = filterH.Handle(context.Background(), monogo.Record{Message: "warning message", Level: monogo.WARNING})
	recs := inner.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Extra["filter_applied"] != true {
		t.Errorf("expected filter_applied=true, got: %v", recs[0].Extra["filter_applied"])
	}
}

func TestGroupHandlerWithProcessor(t *testing.T) {
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := handler.NewTest(monogo.DEBUG)

	groupH := handler.NewGroup([]monogo.Handler{h1, h2},
		handler.WithProcessor(processor.Tag("grouped", true)),
	)

	_ = groupH.Handle(context.Background(), monogo.Record{Message: "grouped message", Level: monogo.INFO})

	if len(h1.Records()) != 1 || len(h2.Records()) != 1 {
		t.Fatalf("expected 1 record in each subhandler, got h1=%d, h2=%d", len(h1.Records()), len(h2.Records()))
	}
	if h1.Records()[0].Extra["grouped"] != true {
		t.Errorf("expected h1 record to have grouped=true, got: %v", h1.Records()[0].Extra["grouped"])
	}
	if h2.Records()[0].Extra["grouped"] != true {
		t.Errorf("expected h2 record to have grouped=true, got: %v", h2.Records()[0].Extra["grouped"])
	}
}

func TestBatchHandlingWithProcessor(t *testing.T) {
	var buf bytes.Buffer
	sh := corehandler.NewStream(&buf, monogo.DEBUG,
		corehandler.WithFormatter(formatter.NewJSON("").WithBatchMode(formatter.BatchModeJSON)),
		corehandler.WithProcessor(processor.Tag("batch_proc", "stream_batch")),
	)

	records := []monogo.Record{
		{Message: "batch msg 1", Level: monogo.INFO},
		{Message: "batch msg 2", Level: monogo.WARNING},
	}

	if err := sh.HandleBatch(context.Background(), records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON batch: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 batch records, got %d", len(parsed))
	}
	for i, item := range parsed {
		extra, ok := item["extra"].(map[string]interface{})
		if !ok || extra["batch_proc"] != "stream_batch" {
			t.Errorf("item %d missing batch_proc='stream_batch', got: %v", i, item["extra"])
		}
	}
}

func TestGroupHandlerHandleBatchWithProcessor(t *testing.T) {
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := handler.NewTest(monogo.DEBUG)

	groupH := handler.NewGroup([]monogo.Handler{h1, h2},
		handler.WithProcessor(processor.Tag("group_batch", "yes")),
	)

	records := []monogo.Record{
		{Message: "b1", Level: monogo.INFO},
		{Message: "b2", Level: monogo.ERROR},
	}

	if err := groupH.HandleBatch(context.Background(), records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, h := range []*corehandler.Test{h1, h2} {
		recs := h.Records()
		if len(recs) != 2 {
			t.Fatalf("expected 2 records, got %d", len(recs))
		}
		for i, r := range recs {
			if r.Extra["group_batch"] != "yes" {
				t.Errorf("record %d missing group_batch='yes', got: %v", i, r.Extra["group_batch"])
			}
		}
	}
}

func TestDeduplicationHandlerBasic(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	// Record 1: ERROR "db failed" at t=0s -> should be handled
	r1 := monogo.Record{
		Message: "db failed",
		Level:   monogo.ERROR,
		Channel: "app",
		Time:    baseTime,
	}
	if err := dedupH.Handle(ctx, r1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(testH.Records()) != 1 {
		t.Fatalf("expected 1 record, got %d", len(testH.Records()))
	}

	// Record 2: Identical ERROR at t=10s (within 60s window) -> should be suppressed
	r2 := monogo.Record{
		Message: "db failed",
		Level:   monogo.ERROR,
		Channel: "app",
		Time:    baseTime.Add(10 * time.Second),
	}
	if err := dedupH.Handle(ctx, r2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(testH.Records()) != 1 {
		t.Fatalf("expected duplicate record to be suppressed, got %d", len(testH.Records()))
	}

	// Record 3: Different ERROR at t=20s -> should be handled
	r3 := monogo.Record{
		Message: "network timeout",
		Level:   monogo.ERROR,
		Channel: "app",
		Time:    baseTime.Add(20 * time.Second),
	}
	if err := dedupH.Handle(ctx, r3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(testH.Records()) != 2 {
		t.Fatalf("expected 2 records, got %d", len(testH.Records()))
	}

	// Record 4: Same message "db failed" but different channel "auth" at t=30s -> should be handled
	r4 := monogo.Record{
		Message: "db failed",
		Level:   monogo.ERROR,
		Channel: "auth",
		Time:    baseTime.Add(30 * time.Second),
	}
	if err := dedupH.Handle(ctx, r4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(testH.Records()) != 3 {
		t.Fatalf("expected 3 records, got %d", len(testH.Records()))
	}

	// Record 5: Same message "db failed" on "app" after window has expired at t=65s -> should be handled
	r5 := monogo.Record{
		Message: "db failed",
		Level:   monogo.ERROR,
		Channel: "app",
		Time:    baseTime.Add(65 * time.Second),
	}
	if err := dedupH.Handle(ctx, r5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(testH.Records()) != 4 {
		t.Fatalf("expected 4 records after window expired, got %d", len(testH.Records()))
	}
}

func TestDeduplicationHandlerBelowLevel(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	// Deduplicate only ERROR and above
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	// 3 identical INFO records within window -> all should pass through
	for i := 0; i < 3; i++ {
		rec := monogo.Record{
			Message: "user clicked button",
			Level:   monogo.INFO,
			Channel: "app",
			Time:    baseTime.Add(time.Duration(i) * time.Second),
		}
		if err := dedupH.Handle(ctx, rec); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if len(testH.Records()) != 3 {
		t.Errorf("expected all 3 INFO records to pass through without deduplication, got %d", len(testH.Records()))
	}
}

func TestDeduplicationHandlerBatch(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	batch := []monogo.Record{
		{Message: "err 1", Level: monogo.ERROR, Channel: "app", Time: baseTime},
		{Message: "err 1", Level: monogo.ERROR, Channel: "app", Time: baseTime.Add(5 * time.Second)}, // duplicate, skip
		{Message: "err 2", Level: monogo.ERROR, Channel: "app", Time: baseTime.Add(10 * time.Second)},
		{Message: "info 1", Level: monogo.INFO, Channel: "app", Time: baseTime.Add(15 * time.Second)},
		{Message: "info 1", Level: monogo.INFO, Channel: "app", Time: baseTime.Add(20 * time.Second)}, // INFO below dedupLevel, keep
	}

	if err := dedupH.HandleBatch(ctx, batch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := testH.Records()
	if len(recs) != 4 {
		t.Fatalf("expected 4 records after batch deduplication (1 duplicate dropped), got %d", len(recs))
	}
	if recs[0].Message != "err 1" || recs[1].Message != "err 2" || recs[2].Message != "info 1" || recs[3].Message != "info 1" {
		t.Errorf("unexpected batch records: %+v", recs)
	}
}

func TestDeduplicationHandlerCustomKey(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	// Deduplicate based on context "error_code"
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second,
		handler.WithDeduplicationKey(func(r monogo.Record) string {
			if code, ok := r.Context["error_code"].(string); ok {
				return code
			}
			return r.Message
		}),
	)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	// Different message, but same error_code -> second should be suppressed
	r1 := monogo.Record{
		Message: "Connection dropped to db-1",
		Level:   monogo.ERROR,
		Context: map[string]interface{}{"error_code": "ERR_DB_DISCONNECT"},
		Time:    baseTime,
	}
	r2 := monogo.Record{
		Message: "Failed to connect to db-2",
		Level:   monogo.ERROR,
		Context: map[string]interface{}{"error_code": "ERR_DB_DISCONNECT"},
		Time:    baseTime.Add(5 * time.Second),
	}

	if err := dedupH.Handle(ctx, r1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := dedupH.Handle(ctx, r2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(testH.Records()) != 1 {
		t.Errorf("expected 1 record due to custom error_code key deduplication, got %d", len(testH.Records()))
	}

	// Verify WithDeduplicationKeyFunc alias
	dedupH2 := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second,
		handler.WithDeduplicationKeyFunc(func(r monogo.Record) string { return r.Message }),
	)
	if dedupH2 == nil {
		t.Fatalf("expected non-nil handler")
	}
}

func TestDeduplicationHandlerWithProcessor(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second,
		handler.WithProcessor(processor.Tag("dedup", "active")),
	)

	ctx := context.Background()
	r := monogo.Record{Message: "test msg", Level: monogo.ERROR}

	if err := dedupH.Handle(ctx, r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := testH.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Extra["dedup"] != "active" {
		t.Errorf("expected extra.dedup='active', got: %v", recs[0].Extra["dedup"])
	}
}

func TestDeduplicationHandlerReset(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	r := monogo.Record{Message: "err", Level: monogo.ERROR, Time: baseTime}

	_ = dedupH.Handle(ctx, r)
	if len(testH.Records()) != 1 {
		t.Fatalf("expected 1 record, got %d", len(testH.Records()))
	}

	// Reset deduplication handler (cascades to inner testH as well)
	if err := dedupH.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from dedupH.Reset: %v", err)
	}
	if len(testH.Records()) != 0 {
		t.Fatalf("expected 0 records after Reset(), got %d", len(testH.Records()))
	}

	// Same record at t=1s should now be handled because store was reset
	r2 := monogo.Record{Message: "err", Level: monogo.ERROR, Time: baseTime.Add(1 * time.Second)}
	_ = dedupH.Handle(ctx, r2)
	if len(testH.Records()) != 1 {
		t.Fatalf("expected 1 record after Reset() and second handle, got %d", len(testH.Records()))
	}
	if !testH.Records()[0].Time.Equal(r2.Time) {
		t.Fatalf("expected record r2 with time %v, got %v", r2.Time, testH.Records()[0].Time)
	}
}

func TestDeduplicationHandlerBubblingAndClose(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second,
		handler.WithBubble(false),
	)

	if dedupH.Bubble() {
		t.Errorf("expected Bubble() to be false when configured with WithBubble(false)")
	}

	if err := dedupH.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error from Close: %v", err)
	}

	if !dedupH.IsHandling(context.Background(), monogo.ERROR) {
		t.Errorf("expected IsHandling to be true for ERROR")
	}
}

func TestDeduplicationHandlerCustomStore(t *testing.T) {
	mockStore := &mockDedupStore{}
	testH := handler.NewTest(monogo.DEBUG)
	dedupH := handler.NewDeduplication(testH, monogo.ERROR, 60*time.Second,
		handler.WithDeduplicationStore(mockStore),
	)

	_ = dedupH.Handle(context.Background(), monogo.Record{Message: "msg", Level: monogo.ERROR})
	if !mockStore.calledIsDuplicate {
		t.Errorf("expected custom store IsDuplicate to be called")
	}

	if err := dedupH.Reset(context.Background()); err != nil {
		t.Fatalf("unexpected error from dedupH.Reset: %v", err)
	}
	if !mockStore.calledReset {
		t.Errorf("expected custom store Reset to be called")
	}
}

func TestWhatFailureGroupHandle_SuppressesErrorsAndPanics(t *testing.T) {
	ctx := context.Background()
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := &failingMockHandler{name: "failing-h2", minLevel: monogo.DEBUG, failHandle: true}
	h3 := &failingMockHandler{name: "panicking-h3", minLevel: monogo.DEBUG, panicHandle: true}
	h3b := &failingMockHandler{name: "panicking-err-h3b", minLevel: monogo.DEBUG, panicHandleError: true}
	h4 := handler.NewTest(monogo.DEBUG)

	var reportedErrors []string
	var reportedHandlers []monogo.Handler

	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{h1, h2, h3, h3b, h4},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
			reportedHandlers = append(reportedHandlers, h)
		}),
	)

	rec := monogo.Record{Message: "test resilience", Level: monogo.INFO}
	if err := wfg.Handle(ctx, rec); err != nil {
		t.Fatalf("expected nil error from WhatFailureGroup.Handle, got: %v", err)
	}

	// Verify healthy handlers received the record despite h2, h3, and h3b failures
	if len(h1.Records()) != 1 || h1.Records()[0].Message != "test resilience" {
		t.Errorf("expected h1 to receive record, got %v", h1.Records())
	}
	if len(h4.Records()) != 1 || h4.Records()[0].Message != "test resilience" {
		t.Errorf("expected h4 to receive record, got %v", h4.Records())
	}

	// Verify callback captured error, string panic, and error object panic
	if len(reportedErrors) != 3 {
		t.Fatalf("expected 3 reported errors, got %d: %v", len(reportedErrors), reportedErrors)
	}
	if !strings.Contains(reportedErrors[0], "handle error simulated: failing-h2") {
		t.Errorf("expected error for h2, got: %s", reportedErrors[0])
	}
	if !strings.Contains(reportedErrors[1], "panic in handler: handle panic simulated: panicking-h3") {
		t.Errorf("expected panic error for h3, got: %s", reportedErrors[1])
	}
	if !strings.Contains(reportedErrors[2], "handle panic error object: panicking-err-h3b") {
		t.Errorf("expected panic error object for h3b, got: %s", reportedErrors[2])
	}
	if reportedHandlers[0] != h2 || reportedHandlers[1] != h3 || reportedHandlers[2] != h3b {
		t.Errorf("reported handler references did not match failing handlers")
	}
}

func TestWhatFailureGroupHandle_CallbackPanicSuppression(t *testing.T) {
	ctx := context.Background()
	hFail := &failingMockHandler{name: "fail", minLevel: monogo.DEBUG, failHandle: true}
	hGood := handler.NewTest(monogo.DEBUG)

	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{hFail, hGood},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			panic("callback exploded intentionally")
		}),
	)

	// Must not panic or return error even if onError callback panics
	if err := wfg.Handle(ctx, monogo.Record{Message: "safe", Level: monogo.INFO}); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if len(hGood.Records()) != 1 {
		t.Errorf("expected hGood to receive record")
	}
}

func TestWhatFailureGroupHandleBatch(t *testing.T) {
	ctx := context.Background()
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := &failingMockHandler{name: "failing-batch", minLevel: monogo.DEBUG, failHandleBatch: true}
	h3 := &failingMockHandler{name: "panicking-batch", minLevel: monogo.DEBUG, panicHandleBatch: true}
	h4 := handler.NewTest(monogo.DEBUG)

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{h1, h2, h3, h4},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	records := []monogo.Record{
		{Message: "batch 1", Level: monogo.INFO},
		{Message: "batch 2", Level: monogo.WARNING},
	}

	if err := wfg.HandleBatch(ctx, records); err != nil {
		t.Fatalf("expected nil error from HandleBatch, got: %v", err)
	}

	if len(h1.Records()) != 2 {
		t.Errorf("expected h1 to receive 2 batch records, got %d", len(h1.Records()))
	}
	if len(h4.Records()) != 2 {
		t.Errorf("expected h4 to receive 2 batch records, got %d", len(h4.Records()))
	}
	if len(reportedErrors) != 2 {
		t.Fatalf("expected 2 reported errors, got %d: %v", len(reportedErrors), reportedErrors)
	}
}

func TestWhatFailureGroupHandleBatch_NonBatchFallback(t *testing.T) {
	ctx := context.Background()
	h1 := &failingNonBatchHandler{name: "healthy-nb", minLevel: monogo.DEBUG}
	h2 := &failingNonBatchHandler{name: "failing-nb", minLevel: monogo.DEBUG, failHandle: true}
	h3 := &failingNonBatchHandler{name: "panicking-nb", minLevel: monogo.DEBUG, panicHandle: true}

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{h1, h2, h3},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	records := []monogo.Record{
		{Message: "rec 1", Level: monogo.INFO},
		{Message: "rec 2", Level: monogo.INFO},
	}

	if err := wfg.HandleBatch(ctx, records); err != nil {
		t.Fatalf("expected nil error from HandleBatch, got: %v", err)
	}

	if len(h1.records) != 2 {
		t.Errorf("expected h1 to receive 2 records, got %d", len(h1.records))
	}
	if h2.handleCalls != 2 {
		t.Errorf("expected h2 to receive 2 handle calls, got %d", h2.handleCalls)
	}
	if h3.handleCalls != 2 {
		t.Errorf("expected h3 to receive 2 handle calls, got %d", h3.handleCalls)
	}
	if len(reportedErrors) != 4 { // 2 failures for h2 + 2 failures for h3
		t.Fatalf("expected 4 reported errors, got %d: %v", len(reportedErrors), reportedErrors)
	}
}

func TestWhatFailureGroupClose(t *testing.T) {
	ctx := context.Background()
	h1 := &failingMockHandler{name: "h1"}
	h2 := &failingMockHandler{name: "h2", failClose: true}
	h3 := &failingMockHandler{name: "h3", panicClose: true}
	h4 := &failingMockHandler{name: "h4"}

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{h1, h2, h3, h4},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	if err := wfg.Close(ctx); err != nil {
		t.Fatalf("expected nil error from Close, got: %v", err)
	}

	if h1.closeCalls != 1 || h2.closeCalls != 1 || h3.closeCalls != 1 || h4.closeCalls != 1 {
		t.Errorf("expected all 4 handlers to have Close called once: h1=%d, h2=%d, h3=%d, h4=%d",
			h1.closeCalls, h2.closeCalls, h3.closeCalls, h4.closeCalls)
	}
	if len(reportedErrors) != 2 {
		t.Fatalf("expected 2 close errors, got %d: %v", len(reportedErrors), reportedErrors)
	}
}

func TestWhatFailureGroupIsHandling(t *testing.T) {
	ctx := context.Background()
	h1 := handler.NewTest(monogo.WARNING)
	h2 := handler.NewTest(monogo.CRITICAL)

	wfg := handler.NewWhatFailureGroup([]monogo.Handler{h1, h2})

	if wfg.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected false for DEBUG")
	}
	if wfg.IsHandling(ctx, monogo.INFO) {
		t.Errorf("expected false for INFO")
	}
	if !wfg.IsHandling(ctx, monogo.WARNING) {
		t.Errorf("expected true for WARNING")
	}
	if !wfg.IsHandling(ctx, monogo.CRITICAL) {
		t.Errorf("expected true for CRITICAL")
	}

	wfgEmpty := handler.NewWhatFailureGroup(nil)
	if wfgEmpty.IsHandling(ctx, monogo.EMERGENCY) {
		t.Errorf("expected false for empty group")
	}
}

func TestWhatFailureGroupIsHandling_PanicSuppression(t *testing.T) {
	ctx := context.Background()
	hPanicking := &failingMockHandler{name: "panic-handling", minLevel: monogo.DEBUG, panicIsHandling: true}
	hNormal := handler.NewTest(monogo.INFO)

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{hPanicking, hNormal},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	// DEBUG: hPanicking panics, hNormal doesn't handle -> false, no crash
	if wfg.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected false for DEBUG")
	}
	if len(reportedErrors) != 1 {
		t.Fatalf("expected 1 reported error from IsHandling panic, got %d", len(reportedErrors))
	}

	// INFO: hPanicking panics, hNormal handles -> true, no crash
	if !wfg.IsHandling(ctx, monogo.INFO) {
		t.Errorf("expected true for INFO")
	}
	if len(reportedErrors) != 2 {
		t.Fatalf("expected 2 reported errors, got %d", len(reportedErrors))
	}
}

func TestWhatFailureGroupHandle_IsHandlingPanicSuppression(t *testing.T) {
	ctx := context.Background()
	hPanicking := &failingMockHandler{name: "panic-handling", minLevel: monogo.DEBUG, panicIsHandling: true}
	hHealthy := handler.NewTest(monogo.DEBUG)

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{hPanicking, hHealthy},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	rec := monogo.Record{Message: "msg", Level: monogo.INFO}
	if err := wfg.Handle(ctx, rec); err != nil {
		t.Fatalf("expected nil error from Handle, got: %v", err)
	}

	if len(hHealthy.Records()) != 1 {
		t.Errorf("expected healthy handler to receive record despite isHandling panic")
	}
	if len(reportedErrors) != 1 {
		t.Errorf("expected 1 reported error from isHandling panic, got %d", len(reportedErrors))
	}
}

func TestWhatFailureGroupHandleBatch_NonBatchIsHandlingPanicSuppression(t *testing.T) {
	ctx := context.Background()
	hPanicking := &failingNonBatchHandler{name: "nb-panic-handling", minLevel: monogo.DEBUG, panicIsHandling: true}
	hHealthy := &failingNonBatchHandler{name: "nb-healthy", minLevel: monogo.DEBUG}

	var reportedErrors []string
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{hPanicking, hHealthy},
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			reportedErrors = append(reportedErrors, err.Error())
		}),
	)

	records := []monogo.Record{
		{Message: "r1", Level: monogo.INFO},
		{Message: "r2", Level: monogo.INFO},
	}
	if err := wfg.HandleBatch(ctx, records); err != nil {
		t.Fatalf("expected nil error from HandleBatch, got: %v", err)
	}

	if len(hHealthy.records) != 2 {
		t.Errorf("expected healthy non-batch handler to receive 2 records, got %d", len(hHealthy.records))
	}
	if len(reportedErrors) != 2 {
		t.Errorf("expected 2 reported errors for the 2 records in batch, got %d", len(reportedErrors))
	}
}

func TestWhatFailureGroupWithProcessor(t *testing.T) {
	ctx := context.Background()
	h1 := handler.NewTest(monogo.DEBUG)
	p1 := processor.Tag("tag1", "val1")
	p2 := processor.Tag("tag2", "val2")

	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{h1},
		handler.WithProcessor(p1),
		handler.WithProcessors(p2),
	)

	if len(wfg.Processors()) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(wfg.Processors()))
	}
	// Verify defensive copy
	wfg.Processors()[0] = nil
	if wfg.Processors()[0] == nil {
		t.Errorf("expected Processors() to return a defensive copy")
	}

	rec := monogo.Record{Message: "msg", Level: monogo.INFO, Extra: make(map[string]interface{})}
	if err := wfg.Handle(ctx, rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Caller record must not be mutated
	if _, ok := rec.Extra["tag1"]; ok {
		t.Errorf("caller record was mutated")
	}

	// Handler received enriched record
	recs := h1.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Extra["tag1"] != "val1" || recs[0].Extra["tag2"] != "val2" {
		t.Errorf("processors were not applied correctly: %v", recs[0].Extra)
	}

	// Test batch with processors
	_ = h1.Reset(ctx)
	batch := []monogo.Record{
		{Message: "b1", Level: monogo.INFO, Extra: make(map[string]interface{})},
		{Message: "b2", Level: monogo.INFO, Extra: make(map[string]interface{})},
	}
	if err := wfg.HandleBatch(ctx, batch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h1.Records()) != 2 {
		t.Fatalf("expected 2 batch records, got %d", len(h1.Records()))
	}
	if h1.Records()[0].Extra["tag1"] != "val1" || h1.Records()[1].Extra["tag2"] != "val2" {
		t.Errorf("batch records not enriched properly: %v", h1.Records())
	}
}

func TestWhatFailureGroupBubblingAndLoggerIntegration(t *testing.T) {
	ctx := context.Background()
	hInner := handler.NewTest(monogo.DEBUG)
	hSubsequent := handler.NewTest(monogo.DEBUG)

	wfg := handler.NewWhatFailureGroup([]monogo.Handler{hInner}, handler.WithBubble(false))
	if wfg.Bubble() {
		t.Errorf("expected Bubble() to be false")
	}

	logger := monogo.New("test-channel", []monogo.Handler{wfg, hSubsequent}, nil)
	if err := logger.Info(ctx, "hello bubbling"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hInner.Records()) != 1 {
		t.Fatalf("expected hInner to receive 1 record, got %d", len(hInner.Records()))
	}
	if len(hSubsequent.Records()) != 0 {
		t.Errorf("expected hSubsequent to receive 0 records due to bubble=false, got %d", len(hSubsequent.Records()))
	}
}

func TestWhatFailureGroup_NotHandledComposition(t *testing.T) {
	ctx := t.Context()

	var callbackCalls int
	onError := func(err error, h monogo.Handler) {
		callbackCalls++
	}

	// Case 1: Sampling handler inside WhatFailureGroup rejects record.
	// ErrNotHandled must NOT be reported to onError callback, and WhatFailureGroup
	// propagates ErrNotHandled so outer fallback receives the record.
	{
		hSampledInner := handler.NewTest(monogo.DEBUG)
		hSampling := handler.NewSampling(hSampledInner, 100, handler.WithSampler(func() bool { return false }))

		wfg := handler.NewWhatFailureGroup([]monogo.Handler{hSampling},
			handler.WithWhatFailureCallback(onError),
			handler.WithBubble(false),
		)
		outerFallback := handler.NewTest(monogo.DEBUG)

		logger := monogo.New("app", []monogo.Handler{wfg, outerFallback}, nil)
		if err := logger.Info(ctx, "rejected record"); err != nil {
			t.Fatalf("unexpected logger error: %v", err)
		}

		if callbackCalls != 0 {
			t.Errorf("expected 0 callback calls for ErrNotHandled sentinel, got %d", callbackCalls)
		}
		if len(outerFallback.Records()) != 1 {
			t.Errorf("expected outerFallback to receive rejected record, got %d", len(outerFallback.Records()))
		}
	}

	// Case 2: One child handles, one rejects.
	// Handled record is not propagated to outer fallback.
	{
		hPrimary := handler.NewTest(monogo.DEBUG)
		hSampledInner := handler.NewTest(monogo.DEBUG)
		hSampling := handler.NewSampling(hSampledInner, 100, handler.WithSampler(func() bool { return false }))

		wfg := handler.NewWhatFailureGroup([]monogo.Handler{hPrimary, hSampling},
			handler.WithWhatFailureCallback(onError),
			handler.WithBubble(false),
		)
		outerFallback := handler.NewTest(monogo.DEBUG)

		logger := monogo.New("app", []monogo.Handler{wfg, outerFallback}, nil)
		if err := logger.Info(ctx, "handled record"); err != nil {
			t.Fatalf("unexpected logger error: %v", err)
		}

		if len(hPrimary.Records()) != 1 {
			t.Errorf("expected hPrimary to receive 1 record, got %d", len(hPrimary.Records()))
		}
		if len(outerFallback.Records()) != 0 {
			t.Errorf("expected outerFallback to receive 0 records, got %d", len(outerFallback.Records()))
		}
	}
}

func TestWhatFailureGroupHandlersInspection(t *testing.T) {
	h1 := handler.NewTest(monogo.DEBUG)
	h2 := handler.NewTest(monogo.INFO)
	wfg := handler.NewWhatFailureGroup([]monogo.Handler{h1, h2})

	handlers := wfg.Handlers()
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(handlers))
	}
	handlers[0] = nil
	if wfg.Handlers()[0] == nil {
		t.Errorf("expected Handlers() to return defensive copy")
	}
}

func TestHandlersImplementResettable(t *testing.T) {
	var _ monogo.Resettable = (*handler.BaseHandler)(nil)
	var _ monogo.Resettable = (*corehandler.Stream)(nil)
	var _ monogo.Resettable = (*handler.Buffer)(nil)
	var _ monogo.Resettable = (*corehandler.FingersCrossed)(nil)
	var _ monogo.Resettable = (*handler.Filter)(nil)
	var _ monogo.Resettable = (*handler.Group)(nil)
	var _ monogo.Resettable = (*handler.WhatFailureGroup)(nil)
	var _ monogo.Resettable = (*handler.Deduplication)(nil)
	var _ monogo.Resettable = (*corehandler.Test)(nil)
	var _ monogo.Resettable = (*corehandler.Null)(nil)
}

func TestBufferHandlerResetAndClear(t *testing.T) {
	ctx := context.Background()
	testH := handler.NewTest(monogo.DEBUG)
	proc := &mockResettableProc{name: "p1"}

	// Buffer with capacity 10, flushLevel ERROR
	bufH := handler.NewBuffer(testH, 10, monogo.ERROR, handler.WithProcessor(proc))

	// Handle 2 DEBUG records (not flushed yet)
	_ = bufH.Handle(ctx, monogo.Record{Message: "msg1", Level: monogo.DEBUG})
	_ = bufH.Handle(ctx, monogo.Record{Message: "msg2", Level: monogo.DEBUG})

	if len(testH.Records()) != 0 {
		t.Fatalf("expected testH to have 0 records before flush/reset, got %d", len(testH.Records()))
	}

	// Calling Reset() flushes buffered records to testH and resets testH + proc
	// But wait: Reset() on testH clears testH's records AFTER receiving flushed records!
	// Let's trace: bufH.Reset() -> b.Flush(ctx) (testH gets msg1, msg2) -> b.BaseHandler.Reset(ctx) (proc.Reset()) -> testH.Reset(ctx) (records cleared!)
	if err := bufH.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from bufH.Reset: %v", err)
	}

	if proc.resetCount != 1 {
		t.Errorf("expected proc resetCount=1, got %d", proc.resetCount)
	}

	// After Reset(), buffer is empty, and testH was also reset
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH records to be cleared by cascading Reset(), got %d", len(testH.Records()))
	}

	// Now log new message after reset; should be buffered
	_ = bufH.Handle(ctx, monogo.Record{Message: "msg3", Level: monogo.DEBUG})
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH to have 0 records before clear/flush, got %d", len(testH.Records()))
	}

	// Test Clear() discards buffered records without sending to testH
	bufH.Clear()
	_ = bufH.Flush(ctx) // Flush should have nothing
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH to have 0 records after Clear(), got %d", len(testH.Records()))
	}
}

func TestFilterHandlerReset(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	proc := &mockResettableProc{name: "p1"}
	filterH := handler.NewFilter(testH, monogo.INFO, monogo.ERROR, handler.WithProcessor(proc))

	ctx := context.Background()
	_ = filterH.Handle(ctx, monogo.Record{Message: "info", Level: monogo.INFO})

	if len(testH.Records()) != 1 {
		t.Fatalf("expected 1 record, got %d", len(testH.Records()))
	}

	if err := filterH.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from filterH.Reset: %v", err)
	}

	if proc.resetCount != 1 {
		t.Errorf("expected proc resetCount=1, got %d", proc.resetCount)
	}
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH records to be cleared after filterH.Reset(), got %d", len(testH.Records()))
	}
}

func TestGroupHandlerReset(t *testing.T) {
	testH1 := handler.NewTest(monogo.DEBUG)
	testH2 := handler.NewTest(monogo.DEBUG)
	proc := &mockResettableProc{name: "p1"}
	groupH := handler.NewGroup([]monogo.Handler{testH1, testH2}, handler.WithProcessor(proc))

	ctx := context.Background()
	_ = groupH.Handle(ctx, monogo.Record{Message: "msg", Level: monogo.INFO})

	if len(testH1.Records()) != 1 || len(testH2.Records()) != 1 {
		t.Fatalf("expected 1 record in each test handler")
	}

	if err := groupH.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from groupH.Reset: %v", err)
	}

	if proc.resetCount != 1 {
		t.Errorf("expected proc resetCount=1, got %d", proc.resetCount)
	}
	if len(testH1.Records()) != 0 || len(testH2.Records()) != 0 {
		t.Errorf("expected both test handlers to be reset")
	}
}

func TestWhatFailureGroupReset_PanicSuppression(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	panicH := &mockPanicResetHandler{Handler: corehandler.NewNull()}
	proc := &mockResettableProc{name: "p1"}

	var callbackErr error
	var callbackHandler monogo.Handler
	wfg := handler.NewWhatFailureGroup(
		[]monogo.Handler{testH, panicH},
		handler.WithProcessor(proc),
		handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
			callbackErr = err
			callbackHandler = h
		}),
	)

	ctx := context.Background()
	_ = testH.Handle(ctx, monogo.Record{Message: "rec", Level: monogo.INFO})

	// Reset should NOT panic despite panicH panicking in Reset()
	_ = wfg.Reset(ctx)

	if proc.resetCount != 1 {
		t.Errorf("expected proc resetCount=1, got %d", proc.resetCount)
	}
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH to be reset")
	}
	if panicH.resetCount != 1 {
		t.Errorf("expected panicH to have had Reset() called")
	}
	if callbackErr == nil {
		t.Errorf("expected callbackErr to be populated with panic error")
	}
	if callbackHandler != panicH {
		t.Errorf("expected callbackHandler to match panicH")
	}
}

func TestBufferHandlerReset_FlushError(t *testing.T) {
	expectedErr := fmt.Errorf("network sink unavailable")
	failH := &failingHandler{err: expectedErr}

	var observedCallbackErr error
	bufH := handler.NewBuffer(
		failH,
		10,
		monogo.ERROR,
		handler.WithResetErrorCallback(func(err error) {
			observedCallbackErr = err
		}),
	)

	ctx := context.Background()
	_ = bufH.Handle(ctx, monogo.Record{Message: "buffered record", Level: monogo.INFO})

	// Before Reset, LastResetError should be nil
	if err := bufH.LastResetError(); err != nil {
		t.Fatalf("expected nil LastResetError before Reset, got %v", err)
	}

	// Trigger Reset
	resetErr := bufH.Reset(ctx)

	// Verify error was observed via returned error, LastResetError(), and callback
	if resetErr != expectedErr {
		t.Errorf("expected Reset(ctx) to return %v, got %v", expectedErr, resetErr)
	}
	if bufH.LastResetError() != expectedErr {
		t.Errorf("expected LastResetError to be %v, got %v", expectedErr, bufH.LastResetError())
	}
	if observedCallbackErr != expectedErr {
		t.Errorf("expected callback to receive %v, got %v", expectedErr, observedCallbackErr)
	}
}

func TestHandlerLifecycleAndReachability(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)

	// Buffer: IsHandling, HandleBatch, Close
	bufH := handler.NewBuffer(testH, 5, monogo.ERROR)
	if !bufH.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected bufH.IsHandling to be true")
	}
	if err := bufH.HandleBatch(ctx, []monogo.Record{{Message: "batch1", Level: monogo.INFO}}); err != nil {
		t.Fatalf("unexpected error from bufH.HandleBatch: %v", err)
	}
	if err := bufH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from bufH.Close: %v", err)
	}

	// Filter: NewFilterFunc, IsHandling, Close
	filterFuncH := handler.NewFilterFunc(testH, func(r monogo.Record) bool {
		return r.Level >= monogo.WARNING
	})
	if !filterFuncH.IsHandling(ctx, monogo.WARNING) {
		t.Errorf("expected filterFuncH.IsHandling to be true")
	}
	if err := filterFuncH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from filterFuncH.Close: %v", err)
	}

	filterH := handler.NewFilter(testH, monogo.INFO, monogo.ERROR)
	if !filterH.IsHandling(ctx, monogo.INFO) {
		t.Errorf("expected filterH.IsHandling to be true")
	}
	if err := filterH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from filterH.Close: %v", err)
	}

	// FingersCrossed: IsHandling, HandleBatch, Close
	fcH := corehandler.NewFingersCrossed(testH, monogo.ERROR, 10)
	if !fcH.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected fcH.IsHandling to be true")
	}
	if err := fcH.HandleBatch(ctx, []monogo.Record{{Message: "fc-batch", Level: monogo.DEBUG}}); err != nil {
		t.Fatalf("unexpected error from fcH.HandleBatch: %v", err)
	}
	if err := fcH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from fcH.Close: %v", err)
	}

	// Group: IsHandling, Close
	groupH := handler.NewGroup([]monogo.Handler{testH})
	if !groupH.IsHandling(ctx, monogo.INFO) {
		t.Errorf("expected groupH.IsHandling to be true")
	}
	if err := groupH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from groupH.Close: %v", err)
	}

	// RotatingFile: WithCompress
	tmpDir := t.TempDir()
	rotH := handler.NewRotatingFile(filepath.Join(tmpDir, "comp.log"), monogo.DEBUG, handler.WithCompress(true))
	defer func() { _ = rotH.Close(ctx) }()
	if !rotH.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected rotH.IsHandling to be true")
	}
}