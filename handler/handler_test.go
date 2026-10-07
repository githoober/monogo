package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

var (
	_ monogo.ProcessableHandler = (*handler.Stream)(nil)
	_ monogo.ProcessableHandler = (*handler.FingersCrossed)(nil)
	_ monogo.ProcessableHandler = (*handler.Test)(nil)
	_ monogo.ProcessableHandler = (*handler.Null)(nil)
	_ monogo.ProcessableHandler = (*handler.JSONStream)(nil)
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

func TestNullAndTestHandler(t *testing.T) {
	nullH := handler.NewNull()
	if err := nullH.Handle(context.Background(), monogo.Record{Message: "test", Level: monogo.DEBUG}); err != nil {
		t.Errorf("null handler handle error: %v", err)
	}

	testH := handler.NewTest(monogo.DEBUG)
	_ = testH.Handle(context.Background(), monogo.Record{Message: "find me", Level: monogo.INFO})

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

func TestHandlerWithProcessorInspection(t *testing.T) {
	p1 := processor.ProcessId()
	p2 := processor.Process()

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
	tagProc := monogo.ProcessorFunc(func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["handler_env"] = "stream_prod"
		return r
	})
	sh := handler.NewStream(&buf, monogo.DEBUG,
		handler.WithFormatter(formatter.NewJSON("")),
		handler.WithProcessor(tagProc),
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
	// h1 has processor tagProc("h1_tag", "one")
	// h2 has processor tagProc("h2_tag", "two")
	// Verifies that mutations made by h1's processor do not leak into h2,
	// and neither leaks back to the logger or other handlers.
	makeTagProc := func(k, v string) monogo.ProcessorFunc {
		return func(r monogo.Record) monogo.Record {
			if r.Extra == nil {
				r.Extra = make(map[string]interface{})
			}
			r.Extra[k] = v
			return r
		}
	}
	h1 := handler.NewTest(monogo.DEBUG, handler.WithProcessor(makeTagProc("h1_tag", "one")))
	h2 := handler.NewTest(monogo.DEBUG, handler.WithProcessor(makeTagProc("h2_tag", "two")))
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

func TestFingersCrossedHandlerWithProcessor(t *testing.T) {
	inner := handler.NewTest(monogo.DEBUG)
	fcTag := monogo.ProcessorFunc(func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["fc_annotated"] = "yes"
		return r
	})
	fcH := handler.NewFingersCrossed(inner, monogo.ERROR, 10,
		handler.WithProcessor(fcTag),
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

func TestFingersCrossedResetAndClear(t *testing.T) {
	ctx := context.Background()
	testH := handler.NewTest(monogo.DEBUG)
	proc := &mockResettableProc{name: "p1"}

	fc := handler.NewFingersCrossed(testH, monogo.ERROR, 10, handler.WithProcessor(proc))

	// Log DEBUG records
	_ = fc.Handle(ctx, monogo.Record{Message: "debug1", Level: monogo.DEBUG})
	_ = fc.Handle(ctx, monogo.Record{Message: "debug2", Level: monogo.DEBUG})

	// Trigger activation
	_ = fc.Handle(ctx, monogo.Record{Message: "error1", Level: monogo.ERROR})

	// testH should have received 3 records
	if len(testH.Records()) != 3 {
		t.Fatalf("expected 3 records, got %d", len(testH.Records()))
	}

	// Reset should disarm triggered flag, clear buffer, call proc.Reset(), and call testH.Reset()
	if err := fc.Reset(ctx); err != nil {
		t.Fatalf("unexpected error from fc.Reset: %v", err)
	}

	if proc.resetCount != 1 {
		t.Errorf("expected proc resetCount=1, got %d", proc.resetCount)
	}
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH records to be cleared after fc.Reset(), got %d", len(testH.Records()))
	}

	// Now log another DEBUG record; should be buffered and NOT sent to testH because fc was disarmed
	_ = fc.Handle(ctx, monogo.Record{Message: "debug3", Level: monogo.DEBUG})
	if len(testH.Records()) != 0 {
		t.Errorf("expected testH to have 0 records, but fc triggered flag was not reset, got %d", len(testH.Records()))
	}

	// Test Clear()
	fc.Clear()
	// Trigger with ERROR again
	_ = fc.Handle(ctx, monogo.Record{Message: "error2", Level: monogo.ERROR})
	// Only error2 should be flushed because debug3 was cleared
	if len(testH.Records()) != 1 {
		t.Errorf("expected 1 record after Clear() and re-trigger, got %d", len(testH.Records()))
	}
}

func TestBaseHandlerReset(t *testing.T) {
	proc1 := &mockResettableProc{name: "p1"}
	proc2 := &mockResettableProc{name: "p2"}
	base := handler.NewBaseHandler(monogo.DEBUG, handler.WithProcessor(proc1, proc2))

	if err := base.Reset(context.Background()); err != nil {
		t.Fatalf("unexpected error from base.Reset: %v", err)
	}

	if proc1.resetCount != 1 || proc2.resetCount != 1 {
		t.Errorf("expected both processors to be reset")
	}

	// Typed nil safety
	var nilBase *handler.BaseHandler
	_ = nilBase.Reset(context.Background()) // should not panic
}


func TestJSONStreamHandler(t *testing.T) {
	var buf bytes.Buffer
	jh := handler.NewJSONStream(&buf, monogo.DEBUG)

	if !jh.IsHandling(context.Background(), monogo.INFO) {
		t.Errorf("JSONStream handler should handle INFO")
	}

	logger := monogo.New("json-stream-app", []monogo.Handler{jh}, nil)
	err := logger.Info(context.Background(), "json stream record", map[string]interface{}{"status": "ready"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if parsed["channel"] != "json-stream-app" {
		t.Errorf("expected channel 'json-stream-app', got %v", parsed["channel"])
	}
	if parsed["message"] != "json stream record" {
		t.Errorf("expected message 'json stream record', got %v", parsed["message"])
	}

	// Test NewJSON alias
	var buf2 bytes.Buffer
	jh2 := handler.NewJSON(&buf2, monogo.WARNING)
	if jh2.IsHandling(context.Background(), monogo.INFO) {
		t.Errorf("jh2 should not handle INFO")
	}
}

func TestCoreHandlerLifecycleAndReachability(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)

	// FingersCrossed: IsHandling, HandleBatch, Close
	fcH := handler.NewFingersCrossed(testH, monogo.ERROR, 10)
	if !fcH.IsHandling(ctx, monogo.DEBUG) {
		t.Errorf("expected fcH.IsHandling to be true")
	}
	if err := fcH.HandleBatch(ctx, []monogo.Record{{Message: "fc-batch", Level: monogo.DEBUG}}); err != nil {
		t.Fatalf("unexpected error from fcH.HandleBatch: %v", err)
	}
	if err := fcH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from fcH.Close: %v", err)
	}

	// Null: Close
	nullH := handler.NewNull()
	if err := nullH.Close(ctx); err != nil {
		t.Fatalf("unexpected error from nullH.Close: %v", err)
	}
}
