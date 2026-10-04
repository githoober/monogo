package monogo_test

import (
	"context"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
)

type mockHandler struct {
	minLevel monogo.Level
	records  []monogo.Record
	closed   bool
	lastCtx  context.Context
}

func (m *mockHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	return level >= m.minLevel
}

func (m *mockHandler) Handle(ctx context.Context, record monogo.Record) error {
	m.lastCtx = ctx
	m.records = append(m.records, record)
	return nil
}

func (m *mockHandler) Close(ctx context.Context) error {
	m.closed = true
	return nil
}

type mockBubblingHandler struct {
	mockHandler
	bubble bool
}

func (m *mockBubblingHandler) Bubble() bool {
	return m.bubble
}

func TestLevelStringsAndParsing(t *testing.T) {
	tests := []struct {
		level monogo.Level
		want  string
	}{
		{monogo.DEBUG, "DEBUG"},
		{monogo.INFO, "INFO"},
		{monogo.NOTICE, "NOTICE"},
		{monogo.WARNING, "WARNING"},
		{monogo.ERROR, "ERROR"},
		{monogo.CRITICAL, "CRITICAL"},
		{monogo.ALERT, "ALERT"},
		{monogo.EMERGENCY, "EMERGENCY"},
		{monogo.Level(999), "LEVEL(999)"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.want {
			t.Errorf("expected string %s, got %s", tt.want, tt.level.String())
		}
	}

	parseTests := []struct {
		input string
		want  monogo.Level
		err   bool
	}{
		{"debug", monogo.DEBUG, false},
		{"INFO", monogo.INFO, false},
		{"notice", monogo.NOTICE, false},
		{"warning", monogo.WARNING, false},
		{"warn", monogo.WARNING, false},
		{"error", monogo.ERROR, false},
		{"err", monogo.ERROR, false},
		{"critical", monogo.CRITICAL, false},
		{"crit", monogo.CRITICAL, false},
		{"alert", monogo.ALERT, false},
		{"emergency", monogo.EMERGENCY, false},
		{"emerg", monogo.EMERGENCY, false},
		{"invalid", monogo.DEBUG, true},
	}

	for _, tt := range parseTests {
		got, err := monogo.ParseLevel(tt.input)
		if tt.err && err == nil {
			t.Errorf("expected error parsing %s, got none", tt.input)
		}
		if !tt.err && (err != nil || got != tt.want) {
			t.Errorf("parse %s expected %v, got %v (err: %v)", tt.input, tt.want, got, err)
		}
	}
}

func TestSlogLevelConversionInAdapter(t *testing.T) {
	if slogadapter.ToSlogLevel(monogo.DEBUG) != -4 {
		t.Errorf("expected -4 for slog.LevelDebug")
	}
}

func TestAmbientContext(t *testing.T) {
	ctx := context.Background()
	ctx = monogo.WithField(ctx, "request_id", "req-123")
	ctx = monogo.WithContext(ctx, map[string]interface{}{"tenant": "acme"})

	ambientMap := monogo.FromContext(ctx)
	if ambientMap["request_id"] != "req-123" {
		t.Errorf("expected request_id req-123, got %v", ambientMap["request_id"])
	}
	if ambientMap["tenant"] != "acme" {
		t.Errorf("expected tenant acme, got %v", ambientMap["tenant"])
	}

	h := &mockHandler{minLevel: monogo.INFO}
	logger := monogo.New("ambient-app", []monogo.Handler{h}, nil)

	err := logger.Info(ctx, "processing order", map[string]interface{}{"order_id": 99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}

	if h.lastCtx != ctx {
		t.Errorf("expected handler to receive client ctx, got %v", h.lastCtx)
	}

	rec := h.records[0]
	if rec.Context["request_id"] != "req-123" {
		t.Errorf("ambient field request_id missing or invalid: %v", rec.Context["request_id"])
	}
	if rec.Context["tenant"] != "acme" {
		t.Errorf("ambient field tenant missing or invalid: %v", rec.Context["tenant"])
	}
	if rec.Context["order_id"] != 99 {
		t.Errorf("explicit field order_id missing or invalid: %v", rec.Context["order_id"])
	}
}

func TestChildLoggerWithNameAndChannel(t *testing.T) {
	h := &mockHandler{minLevel: monogo.INFO}
	parent := monogo.New("parent-channel", []monogo.Handler{h}, nil)

	childName := parent.WithName("child-channel")
	if childName.Name() != "child-channel" {
		t.Errorf("expected child channel 'child-channel', got '%s'", childName.Name())
	}

	childChan := parent.WithChannel("db-channel")
	if childChan.Name() != "db-channel" {
		t.Errorf("expected child channel 'db-channel', got '%s'", childChan.Name())
	}

	_ = childChan.Info(context.Background(), "db query executed")
	if len(h.records) != 1 {
		t.Fatalf("expected 1 record in handler, got %d", len(h.records))
	}
	if h.records[0].Channel != "db-channel" {
		t.Errorf("expected channel 'db-channel', got '%s'", h.records[0].Channel)
	}
}

func TestLoggerPipeline(t *testing.T) {
	h := &mockHandler{minLevel: monogo.INFO}
	logger := monogo.New("app", []monogo.Handler{h}, nil)

	if logger.Name() != "app" {
		t.Errorf("expected channel app, got %s", logger.Name())
	}

	// Should not handle DEBUG
	if err := logger.Debug(context.Background(), "debug msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.records) != 0 {
		t.Errorf("handler should not have received debug record")
	}

	// Should handle INFO
	if err := logger.Info(context.Background(), "info msg", map[string]interface{}{"key": "value"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}

	rec := h.records[0]
	if rec.Message != "info msg" {
		t.Errorf("expected msg 'info msg', got '%s'", rec.Message)
	}
	if rec.Channel != "app" {
		t.Errorf("expected channel 'app', got '%s'", rec.Channel)
	}
	if rec.Context["key"] != "value" {
		t.Errorf("expected context key=value")
	}

	// Test Push/Pop Handlers
	h2 := &mockHandler{minLevel: monogo.DEBUG}
	logger.PushHandler(h2)

	if err := logger.Debug(context.Background(), "debug msg 2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h2.records) != 1 {
		t.Errorf("expected h2 to receive debug record")
	}

	popped, ok := logger.PopHandler()
	if !ok || popped != h2 {
		t.Errorf("expected to pop h2")
	}

	// Test With child logger
	child := logger.With(map[string]interface{}{"env": "production"})
	if err := child.Info(context.Background(), "child msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lastRec := h.records[len(h.records)-1]
	if lastRec.Context["env"] != "production" {
		t.Errorf("expected child context env=production, got %v", lastRec.Context)
	}

	// Test Close
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("failed to close: %v", err)
	}
	if !h.closed {
		t.Errorf("expected handler to be closed")
	}
}

func TestHandlerBubblingStopsPropagation(t *testing.T) {
	// Top handler: handles ERROR+, bubble = false
	topHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monogo.ERROR},
		bubble:      false,
	}

	// Bottom handler: handles DEBUG+, bubble = true
	bottomHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monogo.DEBUG},
		bubble:      true,
	}

	logger := monogo.New("bubble-test", []monogo.Handler{topHandler, bottomHandler}, nil)

	// 1. Log INFO: topHandler does NOT handle it, so it should bypass topHandler and reach bottomHandler
	if err := logger.Info(context.Background(), "info message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topHandler.records) != 0 {
		t.Errorf("top handler should not have received info message")
	}
	if len(bottomHandler.records) != 1 {
		t.Fatalf("bottom handler should have received info message, got %d", len(bottomHandler.records))
	}

	// 2. Log ERROR: topHandler handles it and bubble = false, so bottomHandler should NOT receive it
	if err := logger.Error(context.Background(), "error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topHandler.records) != 1 {
		t.Fatalf("top handler should have received error message, got %d", len(topHandler.records))
	}
	if len(bottomHandler.records) != 1 {
		t.Errorf("bottom handler should NOT have received error message due to bubble=false, got %d", len(bottomHandler.records))
	}
}

func TestHandlerBubblingContinuesWhenTrue(t *testing.T) {
	// Top handler: handles ERROR+, bubble = true
	topHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monogo.ERROR},
		bubble:      true,
	}

	// Bottom handler: handles DEBUG+, bubble = true
	bottomHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monogo.DEBUG},
		bubble:      true,
	}

	logger := monogo.New("bubble-test-continue", []monogo.Handler{topHandler, bottomHandler}, nil)

	// Log ERROR: both handlers handle it because topHandler has bubble = true
	if err := logger.Error(context.Background(), "error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topHandler.records) != 1 {
		t.Fatalf("expected top handler to have 1 record, got %d", len(topHandler.records))
	}
	if len(bottomHandler.records) != 1 {
		t.Fatalf("expected bottom handler to have 1 record, got %d", len(bottomHandler.records))
	}
}

type contextAwareHandler struct {
	handledRecords []monogo.Record
}

func (c *contextAwareHandler) IsHandling(ctx context.Context, level monogo.Level) bool {
	if level >= monogo.INFO {
		return true
	}
	// Dynamically allow DEBUG only if context has "debug_mode" == true
	if val, ok := ctx.Value("debug_mode").(bool); ok && val {
		return true
	}
	return false
}

func (c *contextAwareHandler) Handle(ctx context.Context, record monogo.Record) error {
	c.handledRecords = append(c.handledRecords, record)
	return nil
}

func (c *contextAwareHandler) Close(ctx context.Context) error {
	return nil
}

func TestLoggerIsHandlingContext(t *testing.T) {
	h := &contextAwareHandler{}
	logger := monogo.New("ctx-handling-test", []monogo.Handler{h}, nil)

	ctxOff := context.Background()
	ctxOn := context.WithValue(context.Background(), "debug_mode", true)

	// Test IsHandling directly
	if logger.IsHandling(ctxOff, monogo.DEBUG) {
		t.Errorf("expected IsHandling to be false when debug_mode is absent")
	}
	if !logger.IsHandling(ctxOn, monogo.DEBUG) {
		t.Errorf("expected IsHandling to be true when debug_mode is true")
	}
	if !logger.IsHandling(ctxOff, monogo.INFO) {
		t.Errorf("expected IsHandling to be true for INFO regardless of debug_mode")
	}

	// Test Log early return based on context
	if err := logger.Debug(ctxOff, "disabled debug"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.handledRecords) != 0 {
		t.Errorf("expected 0 records logged when debug is disabled in ctx, got %d", len(h.handledRecords))
	}

	if err := logger.Debug(ctxOn, "enabled debug"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.handledRecords) != 1 {
		t.Errorf("expected 1 record logged when debug is enabled in ctx, got %d", len(h.handledRecords))
	}
}

type mockResettableHandler struct {
	mockHandler
	resetCount int
}

func (m *mockResettableHandler) Reset() {
	m.resetCount++
	m.records = nil
}

type mockResettableProcessor struct {
	resetCount int
	tag        string
}

func (m *mockResettableProcessor) Process(r monogo.Record) monogo.Record {
	if r.Extra == nil {
		r.Extra = make(map[string]interface{})
	}
	r.Extra["tag"] = m.tag
	return r
}

func (m *mockResettableProcessor) Reset() {
	m.resetCount++
	m.tag = "reset"
}

func TestLoggerReset_CascadesToHandlersAndProcessors(t *testing.T) {
	rh1 := &mockResettableHandler{mockHandler: mockHandler{minLevel: monogo.DEBUG}}
	rh2 := &mockResettableHandler{mockHandler: mockHandler{minLevel: monogo.DEBUG}}
	plainH := &mockHandler{minLevel: monogo.DEBUG}

	rp := &mockResettableProcessor{tag: "initial"}
	plainP := monogo.ProcessorFunc(func(r monogo.Record) monogo.Record { return r })

	logger := monogo.New("reset-test", []monogo.Handler{rh1, plainH, rh2}, []monogo.Processor{rp, plainP})

	ctx := context.Background()
	_ = logger.Info(ctx, "before reset")

	if rh1.resetCount != 0 || rh2.resetCount != 0 || rp.resetCount != 0 {
		t.Fatalf("expected 0 resets before calling Reset()")
	}

	// Call Reset on Logger
	logger.Reset()

	if rh1.resetCount != 1 {
		t.Errorf("expected rh1 resetCount=1, got %d", rh1.resetCount)
	}
	if rh2.resetCount != 1 {
		t.Errorf("expected rh2 resetCount=1, got %d", rh2.resetCount)
	}
	if rp.resetCount != 1 {
		t.Errorf("expected rp resetCount=1, got %d", rp.resetCount)
	}
	if rp.tag != "reset" {
		t.Errorf("expected rp.tag to be 'reset', got %s", rp.tag)
	}

	// Verify that rh1's records slice was cleared
	if len(rh1.records) != 0 {
		t.Errorf("expected rh1 records to be cleared after reset, got %d", len(rh1.records))
	}
}

func TestLoggerReset_EmptyLoggerDoesNotPanic(t *testing.T) {
	emptyLogger := monogo.New("empty", nil, nil)
	// Should not panic
	emptyLogger.Reset()
}

func TestLoggerImplementsResettable(t *testing.T) {
	var _ monogo.Resettable = (*monogo.Logger)(nil)
	l := monogo.New("test", nil, nil)
	var r monogo.Resettable = l
	r.Reset()
}

