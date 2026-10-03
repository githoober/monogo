package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func TestStreamHandler(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.INFO)

	if !sh.IsHandling(context.Background(), monogo.INFO) {
		t.Errorf("Stream handler should handle INFO")
	}
	if sh.IsHandling(context.Background(), monogo.DEBUG) {
		t.Errorf("Stream handler should not handle DEBUG")
	}

	rec := monogo.Record{
		Message: "stream test",
		Level:   monogo.INFO,
		Channel: "app",
	}

	if err := sh.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Stream handle error: %v", err)
	}

	if !strings.Contains(buf.String(), "app.INFO: stream test") {
		t.Errorf("Unexpected stream output: %s", buf.String())
	}
}

func TestStreamHandlerJSONFile(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.DEBUG, handler.WithFormatter(formatter.NewJSON("")))

	logger := monogo.New("json-file-app", []monogo.Handler{sh}, nil)

	err := logger.Info(context.Background(), "writing json logs", map[string]interface{}{"file": "app.log", "status": "ok"})
	if err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json output: %v (raw: %s)", err, buf.String())
	}

	if parsed["channel"] != "json-file-app" {
		t.Errorf("expected channel json-file-app, got %v", parsed["channel"])
	}
	if parsed["message"] != "writing json logs" {
		t.Errorf("expected message 'writing json logs', got %v", parsed["message"])
	}

	ctx, ok := parsed["context"].(map[string]interface{})
	if !ok || ctx["file"] != "app.log" {
		t.Errorf("expected context file=app.log, got %v", parsed["context"])
	}
}

func TestRotatingFileHandler(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotating.log")

	rotH := handler.NewRotatingFile(logPath, monogo.INFO,
		handler.WithMaxSize(1),
		handler.WithMaxBackups(2),
		handler.WithMaxAge(7),
	)
	defer rotH.Close(context.Background())

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
	defer rotH.Close(context.Background())

	if rotH.Bubble() {
		t.Errorf("expected Bubble to be false with WithBubble(false)")
	}
}

func TestFingersCrossedHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	fc := handler.NewFingersCrossed(testH, monogo.ERROR, 10)

	_ = fc.Handle(context.Background(), monogo.Record{Message: "debug 1", Level: monogo.DEBUG})
	_ = fc.Handle(context.Background(), monogo.Record{Message: "info 1", Level: monogo.INFO})

	if len(testH.Records()) != 0 {
		t.Fatalf("FingersCrossed should not have flushed records yet")
	}

	_ = fc.Handle(context.Background(), monogo.Record{Message: "error 1", Level: monogo.ERROR})

	recs := testH.Records()
	if len(recs) != 3 {
		t.Fatalf("Expected 3 records after error trigger, got %d", len(recs))
	}
	if recs[0].Message != "debug 1" || recs[1].Message != "info 1" || recs[2].Message != "error 1" {
		t.Errorf("Unexpected flushed records order/content: %v", recs)
	}

	_ = fc.Handle(context.Background(), monogo.Record{Message: "debug 2 post-trigger", Level: monogo.DEBUG})
	if len(testH.Records()) != 4 {
		t.Errorf("Expected 4 records after post-trigger log, got %d", len(testH.Records()))
	}
}

func TestFilterHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	filterH := handler.NewFilter(testH, monogo.INFO, monogo.ERROR)

	filterH.Handle(context.Background(), monogo.Record{Message: "debug", Level: monogo.DEBUG})
	filterH.Handle(context.Background(), monogo.Record{Message: "info", Level: monogo.INFO})
	filterH.Handle(context.Background(), monogo.Record{Message: "crit", Level: monogo.CRITICAL})

	recs := testH.Records()
	if len(recs) != 1 || recs[0].Message != "info" {
		t.Errorf("Filter handler failed, expected only 'info', got: %v", recs)
	}
}

func TestGroupHandler(t *testing.T) {
	t1 := handler.NewTest(monogo.DEBUG)
	t2 := handler.NewTest(monogo.WARNING)
	group := handler.NewGroup([]monogo.Handler{t1, t2})

	group.Handle(context.Background(), monogo.Record{Message: "info msg", Level: monogo.INFO})
	group.Handle(context.Background(), monogo.Record{Message: "warn msg", Level: monogo.WARNING})

	if len(t1.Records()) != 2 {
		t.Errorf("t1 should have 2 records, got %d", len(t1.Records()))
	}
	if len(t2.Records()) != 1 {
		t.Errorf("t2 should have 1 record, got %d", len(t2.Records()))
	}
}

func TestBufferHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	bufH := handler.NewBuffer(testH, 3, monogo.ERROR)

	bufH.Handle(context.Background(), monogo.Record{Message: "msg 1", Level: monogo.INFO})
	bufH.Handle(context.Background(), monogo.Record{Message: "msg 2", Level: monogo.INFO})

	if len(testH.Records()) != 0 {
		t.Errorf("buffer should not have flushed yet")
	}

	bufH.Handle(context.Background(), monogo.Record{Message: "msg 3 error", Level: monogo.ERROR})
	if len(testH.Records()) != 3 {
		t.Errorf("buffer should have flushed 3 records, got %d", len(testH.Records()))
	}
}

func TestNullAndTestHandler(t *testing.T) {
	nullH := handler.NewNull()
	if err := nullH.Handle(context.Background(), monogo.Record{Message: "test", Level: monogo.DEBUG}); err != nil {
		t.Errorf("null handler handle error: %v", err)
	}

	testH := handler.NewTest(monogo.DEBUG)
	testH.Handle(context.Background(), monogo.Record{Message: "find me", Level: monogo.INFO})

	found := testH.HasRecord(func(r monogo.Record) bool {
		return r.Message == "find me"
	})
	if !found {
		t.Errorf("Test handler HasRecord failed to find record")
	}
}

func TestBaseHandlerBubble(t *testing.T) {
	// Default bubbling (no options)
	bhDefault := handler.NewBaseHandler(monogo.INFO)
	if !bhDefault.Bubble() {
		t.Errorf("expected default bubble to be true")
	}

	// Explicit bubbling = false using WithBubble option
	bhNoBubble := handler.NewBaseHandler(monogo.INFO, handler.WithBubble(false))
	if bhNoBubble.Bubble() {
		t.Errorf("expected bubble to be false when configured with WithBubble(false)")
	}
}

func TestStreamHandlerBubbling(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	// sh1 configured with bubble = false using WithBubble option
	sh1 := handler.NewStream(&buf1, monogo.ERROR, handler.WithBubble(false))
	sh2 := handler.NewStream(&buf2, monogo.DEBUG)

	logger := monogo.New("bubble-stream-test", []monogo.Handler{sh1, sh2}, nil)

	// INFO: sh1 does not handle, so sh2 receives it
	if err := logger.Info(context.Background(), "info msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf1.Len() != 0 {
		t.Errorf("expected buf1 to be empty, got: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "info msg") {
		t.Errorf("expected buf2 to contain info msg, got: %s", buf2.String())
	}

	buf2.Reset()

	// ERROR: sh1 handles it and stops bubbling (sh1.Bubble() == false), sh2 should not receive it
	if err := logger.Error(context.Background(), "error msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf1.String(), "error msg") {
		t.Errorf("expected buf1 to contain error msg, got: %s", buf1.String())
	}
	if buf2.Len() != 0 {
		t.Errorf("expected buf2 to be empty due to bubble=false on sh1, got: %s", buf2.String())
	}
}

type batchTrackingHandler struct {
	*handler.Test
	batchCalls int
}

func newBatchTrackingHandler(minLevel monogo.Level) *batchTrackingHandler {
	return &batchTrackingHandler{
		Test: handler.NewTest(minLevel),
	}
}

func (b *batchTrackingHandler) HandleBatch(ctx context.Context, records []monogo.Record) error {
	b.batchCalls++
	return b.Test.HandleBatch(ctx, records)
}

func TestStreamHandlerHandleBatch(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.INFO)

	records := []monogo.Record{
		{Message: "debug message", Level: monogo.DEBUG}, // ignored
		{Message: "info message", Level: monogo.INFO},   // handled
		{Message: "error message", Level: monogo.ERROR}, // handled
	}

	if err := sh.HandleBatch(context.Background(), records); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "debug message") {
		t.Errorf("expected debug message to be skipped by minLevel")
	}
	if !strings.Contains(out, "info message") {
		t.Errorf("expected info message to be written")
	}
	if !strings.Contains(out, "error message") {
		t.Errorf("expected error message to be written")
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

func TestFingersCrossedFlushesViaHandleBatch(t *testing.T) {
	inner := newBatchTrackingHandler(monogo.DEBUG)
	fc := handler.NewFingersCrossed(inner, monogo.ERROR, 10)

	_ = fc.Handle(context.Background(), monogo.Record{Message: "step 1", Level: monogo.INFO})
	_ = fc.Handle(context.Background(), monogo.Record{Message: "step 2", Level: monogo.WARNING})

	if inner.batchCalls != 0 {
		t.Fatalf("expected 0 batch calls before activation, got %d", inner.batchCalls)
	}

	// Trigger activation via ERROR
	_ = fc.Handle(context.Background(), monogo.Record{Message: "failure", Level: monogo.ERROR})

	if inner.batchCalls != 1 {
		t.Errorf("expected exactly 1 HandleBatch call on trigger, got %d", inner.batchCalls)
	}
	if len(inner.Records()) != 3 {
		t.Errorf("expected 3 records flushed, got %d", len(inner.Records()))
	}
}

func TestFilterHandlerHandleBatch(t *testing.T) {
	inner := newBatchTrackingHandler(monogo.DEBUG)
	filter := handler.NewFilter(inner, monogo.INFO, monogo.WARNING)

	records := []monogo.Record{
		{Message: "debug", Level: monogo.DEBUG},     // filtered out
		{Message: "info", Level: monogo.INFO},       // passes
		{Message: "warn", Level: monogo.WARNING},   // passes
		{Message: "error", Level: monogo.ERROR},     // filtered out
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

type nonBatchMockHandler struct {
	minLevel    monogo.Level
	records     []monogo.Record
	handleCalls int
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

// Ensure all handlers implement monogo.ProcessableHandler at compile time
var (
	_ monogo.ProcessableHandler = (*handler.Stream)(nil)
	_ monogo.ProcessableHandler = (*handler.Buffer)(nil)
	_ monogo.ProcessableHandler = (*handler.Filter)(nil)
	_ monogo.ProcessableHandler = (*handler.Group)(nil)
	_ monogo.ProcessableHandler = (*handler.FingersCrossed)(nil)
	_ monogo.ProcessableHandler = (*handler.Test)(nil)
	_ monogo.ProcessableHandler = (*handler.Null)(nil)
)

func TestHandlerWithProcessorInspection(t *testing.T) {
	p1 := processor.Tag("tag1", "val1")
	p2 := processor.Tag("tag2", "val2")

	sh := handler.NewStream(&bytes.Buffer{}, monogo.DEBUG, handler.WithProcessor(p1), handler.WithProcessors(p2))

	procs := sh.Processors()
	if len(procs) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(procs))
	}

	// Verify mutating returned slice does not alter handler's internal processor list
	procs[0] = nil
	if sh.Processors()[0] == nil {
		t.Errorf("expected Processors() to return a copy, not an internal reference")
	}
}

func TestStreamHandlerWithProcessor(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.DEBUG,
		handler.WithFormatter(formatter.NewJSON("")),
		handler.WithProcessor(processor.Tag("handler_env", "stream_prod")),
	)

	logger := monogo.New("test", []monogo.Handler{sh}, nil)
	if err := logger.Info(context.Background(), "stream processor test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	extra, ok := data["extra"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extra map in output, got: %v", data["extra"])
	}
	if extra["handler_env"] != "stream_prod" {
		t.Errorf("expected extra.handler_env = 'stream_prod', got: %v", extra["handler_env"])
	}
}

func TestHandlerProcessorIsolation(t *testing.T) {
	// Two handlers on the same logger pipeline:
	// h1 has processor Tag("target", "handler_1")
	// h2 has processor Tag("target", "handler_2")
	// Verifies that mutations made by h1's processor do not leak into h2,
	// and neither leaks back to the logger or other handlers.
	h1 := handler.NewTest(monogo.DEBUG, handler.WithProcessor(processor.Tag("h1_tag", "one")))
	h2 := handler.NewTest(monogo.DEBUG, handler.WithProcessor(processor.Tag("h2_tag", "two")))
	h3 := handler.NewTest(monogo.DEBUG) // No processors

	logger := monogo.New("isolation-test", []monogo.Handler{h1, h2, h3}, nil)

	if err := logger.Info(context.Background(), "message to all handlers"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs1 := h1.Records()
	recs2 := h2.Records()
	recs3 := h3.Records()

	if len(recs1) != 1 || len(recs2) != 1 || len(recs3) != 1 {
		t.Fatalf("expected 1 record in each handler, got h1=%d, h2=%d, h3=%d", len(recs1), len(recs2), len(recs3))
	}

	r1 := recs1[0]
	r2 := recs2[0]
	r3 := recs3[0]

	// h1 must have h1_tag but NOT h2_tag
	if r1.Extra["h1_tag"] != "one" {
		t.Errorf("expected r1 to have h1_tag='one', got: %v", r1.Extra["h1_tag"])
	}
	if _, exists := r1.Extra["h2_tag"]; exists {
		t.Errorf("r1 unexpectedly has h2_tag: %v", r1.Extra["h2_tag"])
	}

	// h2 must have h2_tag but NOT h1_tag
	if r2.Extra["h2_tag"] != "two" {
		t.Errorf("expected r2 to have h2_tag='two', got: %v", r2.Extra["h2_tag"])
	}
	if _, exists := r2.Extra["h1_tag"]; exists {
		t.Errorf("r2 unexpectedly has h1_tag: %v", r2.Extra["h1_tag"])
	}

	// h3 must have NO extra tags
	if _, exists := r3.Extra["h1_tag"]; exists {
		t.Errorf("r3 unexpectedly contaminated with h1_tag: %v", r3.Extra["h1_tag"])
	}
	if _, exists := r3.Extra["h2_tag"]; exists {
		t.Errorf("r3 unexpectedly contaminated with h2_tag: %v", r3.Extra["h2_tag"])
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

func TestFingersCrossedHandlerWithProcessor(t *testing.T) {
	inner := handler.NewTest(monogo.DEBUG)
	fcH := handler.NewFingersCrossed(inner, monogo.ERROR, 10,
		handler.WithProcessor(processor.Tag("fc_annotated", "yes")),
	)

	_ = fcH.Handle(context.Background(), monogo.Record{Message: "debug 1", Level: monogo.DEBUG})
	_ = fcH.Handle(context.Background(), monogo.Record{Message: "info 2", Level: monogo.INFO})
	if len(inner.Records()) != 0 {
		t.Errorf("expected 0 records before trigger, got %d", len(inner.Records()))
	}

	// Trigger with ERROR
	_ = fcH.Handle(context.Background(), monogo.Record{Message: "error 3", Level: monogo.ERROR})
	recs := inner.Records()
	if len(recs) != 3 {
		t.Fatalf("expected 3 records after trigger, got %d", len(recs))
	}
	for i, r := range recs {
		if r.Extra["fc_annotated"] != "yes" {
			t.Errorf("record %d missing fc_annotated, got: %v", i, r.Extra["fc_annotated"])
		}
	}
}

func TestBatchHandlingWithProcessor(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.DEBUG,
		handler.WithFormatter(formatter.NewJSON("").WithBatchMode(formatter.BatchModeJSON)),
		handler.WithProcessor(processor.Tag("batch_proc", "stream_batch")),
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

	for _, h := range []*handler.Test{h1, h2} {
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



