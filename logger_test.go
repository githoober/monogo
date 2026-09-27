package monolog_test

import (
	"context"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
)

type mockHandler struct {
	minLevel monolog.Level
	records  []monolog.Record
	closed   bool
}

func (m *mockHandler) IsHandling(level monolog.Level) bool {
	return level >= m.minLevel
}

func (m *mockHandler) Handle(record monolog.Record) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockHandler) Close() error {
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
		level monolog.Level
		want  string
	}{
		{monolog.DEBUG, "DEBUG"},
		{monolog.INFO, "INFO"},
		{monolog.NOTICE, "NOTICE"},
		{monolog.WARNING, "WARNING"},
		{monolog.ERROR, "ERROR"},
		{monolog.CRITICAL, "CRITICAL"},
		{monolog.ALERT, "ALERT"},
		{monolog.EMERGENCY, "EMERGENCY"},
		{monolog.Level(999), "LEVEL(999)"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.want {
			t.Errorf("expected string %s, got %s", tt.want, tt.level.String())
		}
	}

	parseTests := []struct {
		input string
		want  monolog.Level
		err   bool
	}{
		{"debug", monolog.DEBUG, false},
		{"INFO", monolog.INFO, false},
		{"notice", monolog.NOTICE, false},
		{"warning", monolog.WARNING, false},
		{"warn", monolog.WARNING, false},
		{"error", monolog.ERROR, false},
		{"err", monolog.ERROR, false},
		{"critical", monolog.CRITICAL, false},
		{"crit", monolog.CRITICAL, false},
		{"alert", monolog.ALERT, false},
		{"emergency", monolog.EMERGENCY, false},
		{"emerg", monolog.EMERGENCY, false},
		{"invalid", monolog.DEBUG, true},
	}

	for _, tt := range parseTests {
		got, err := monolog.ParseLevel(tt.input)
		if tt.err && err == nil {
			t.Errorf("expected error parsing %s, got none", tt.input)
		}
		if !tt.err && (err != nil || got != tt.want) {
			t.Errorf("parse %s expected %v, got %v (err: %v)", tt.input, tt.want, got, err)
		}
	}
}

func TestSlogLevelConversionInAdapter(t *testing.T) {
	if slogadapter.ToSlogLevel(monolog.DEBUG) != -4 {
		t.Errorf("expected -4 for slog.LevelDebug")
	}
}

func TestAmbientContext(t *testing.T) {
	ctx := context.Background()
	ctx = monolog.WithField(ctx, "request_id", "req-123")
	ctx = monolog.WithContext(ctx, map[string]interface{}{"tenant": "acme"})

	ambientMap := monolog.FromContext(ctx)
	if ambientMap["request_id"] != "req-123" {
		t.Errorf("expected request_id req-123, got %v", ambientMap["request_id"])
	}
	if ambientMap["tenant"] != "acme" {
		t.Errorf("expected tenant acme, got %v", ambientMap["tenant"])
	}

	h := &mockHandler{minLevel: monolog.INFO}
	logger := monolog.New("ambient-app", []monolog.Handler{h}, nil)

	err := logger.InfoContext(ctx, "processing order", map[string]interface{}{"order_id": 99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
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
	h := &mockHandler{minLevel: monolog.INFO}
	parent := monolog.New("parent-channel", []monolog.Handler{h}, nil)

	childName := parent.WithName("child-channel")
	if childName.Name() != "child-channel" {
		t.Errorf("expected child channel 'child-channel', got '%s'", childName.Name())
	}

	childChan := parent.WithChannel("db-channel")
	if childChan.Name() != "db-channel" {
		t.Errorf("expected child channel 'db-channel', got '%s'", childChan.Name())
	}

	_ = childChan.Info("db query executed")
	if len(h.records) != 1 {
		t.Fatalf("expected 1 record in handler, got %d", len(h.records))
	}
	if h.records[0].Channel != "db-channel" {
		t.Errorf("expected channel 'db-channel', got '%s'", h.records[0].Channel)
	}
}

func TestLoggerPipeline(t *testing.T) {
	h := &mockHandler{minLevel: monolog.INFO}
	logger := monolog.New("app", []monolog.Handler{h}, nil)

	if logger.Name() != "app" {
		t.Errorf("expected channel app, got %s", logger.Name())
	}

	// Should not handle DEBUG
	if err := logger.Debug("debug msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.records) != 0 {
		t.Errorf("handler should not have received debug record")
	}

	// Should handle INFO
	if err := logger.Info("info msg", map[string]interface{}{"key": "value"}); err != nil {
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
	h2 := &mockHandler{minLevel: monolog.DEBUG}
	logger.PushHandler(h2)

	if err := logger.Debug("debug msg 2"); err != nil {
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
	if err := child.Info("child msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lastRec := h.records[len(h.records)-1]
	if lastRec.Context["env"] != "production" {
		t.Errorf("expected child context env=production, got %v", lastRec.Context)
	}

	// Test Close
	if err := logger.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}
	if !h.closed {
		t.Errorf("expected handler to be closed")
	}
}

func TestHandlerBubblingStopsPropagation(t *testing.T) {
	// Top handler: handles ERROR+, bubble = false
	topHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monolog.ERROR},
		bubble:      false,
	}

	// Bottom handler: handles DEBUG+, bubble = true
	bottomHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monolog.DEBUG},
		bubble:      true,
	}

	logger := monolog.New("bubble-test", []monolog.Handler{topHandler, bottomHandler}, nil)

	// 1. Log INFO: topHandler does NOT handle it, so it should bypass topHandler and reach bottomHandler
	if err := logger.Info("info message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topHandler.records) != 0 {
		t.Errorf("top handler should not have received info message")
	}
	if len(bottomHandler.records) != 1 {
		t.Fatalf("bottom handler should have received info message, got %d", len(bottomHandler.records))
	}

	// 2. Log ERROR: topHandler handles it and bubble = false, so bottomHandler should NOT receive it
	if err := logger.Error("error message"); err != nil {
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
		mockHandler: mockHandler{minLevel: monolog.ERROR},
		bubble:      true,
	}

	// Bottom handler: handles DEBUG+, bubble = true
	bottomHandler := &mockBubblingHandler{
		mockHandler: mockHandler{minLevel: monolog.DEBUG},
		bubble:      true,
	}

	logger := monolog.New("bubble-test-continue", []monolog.Handler{topHandler, bottomHandler}, nil)

	// Log ERROR: both handlers handle it because topHandler has bubble = true
	if err := logger.Error("error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topHandler.records) != 1 {
		t.Fatalf("expected top handler to have 1 record, got %d", len(topHandler.records))
	}
	if len(bottomHandler.records) != 1 {
		t.Fatalf("expected bottom handler to have 1 record, got %d", len(bottomHandler.records))
	}
}

