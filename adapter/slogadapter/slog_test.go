package slogadapter_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func TestSlogHandler(t *testing.T) {
	var buf bytes.Buffer
	slogH := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	monoH := slogadapter.NewSlogHandler(slogH, monogo.DEBUG)

	logger := monogo.New("test-channel", []monogo.Handler{monoH}, nil)

	err := logger.Info(context.Background(), "hello slog backend", map[string]interface{}{"user": "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `msg="hello slog backend"`) {
		t.Errorf("expected msg in output, got: %s", out)
	}
	if !strings.Contains(out, `channel=test-channel`) {
		t.Errorf("expected channel in output, got: %s", out)
	}
	if !strings.Contains(out, `user=alice`) {
		t.Errorf("expected context user=alice in output, got: %s", out)
	}
}

func TestMonologSlogBridge(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	monoLogger := monogo.New("app-channel", []monogo.Handler{testH}, nil)

	slogBridge := slogadapter.NewMonologSlogBridge(monoLogger)
	slogger := slog.New(slogBridge)

	slogger.InfoContext(context.Background(), "hello from slog frontend", slog.String("key", "val"))

	recs := testH.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	rec := recs[0]
	if rec.Message != "hello from slog frontend" {
		t.Errorf("unexpected message: %s", rec.Message)
	}
	if rec.Level != monogo.INFO {
		t.Errorf("unexpected level: %v", rec.Level)
	}
	if rec.Context["key"] != "val" {
		t.Errorf("unexpected context val: %v", rec.Context["key"])
	}
}

func TestSlogHandlerBubbling(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	slogH1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelError})
	slogH2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})

	h1 := slogadapter.NewSlogHandler(slogH1, monogo.ERROR, handler.WithBubble(false))
	h2 := slogadapter.NewSlogHandler(slogH2, monogo.DEBUG)

	if !h2.Bubble() {
		t.Errorf("expected h2 default bubble to be true")
	}
	if h1.Bubble() {
		t.Errorf("expected h1 bubble to be false with WithBubble(false)")
	}

	logger := monogo.New("slog-bubble-test", []monogo.Handler{h1, h2}, nil)

	// INFO: h1 ignores, h2 receives
	if err := logger.Info(context.Background(), "info message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf1.Len() != 0 {
		t.Errorf("expected buf1 to be empty, got: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "info message") {
		t.Errorf("expected buf2 to contain info message, got: %s", buf2.String())
	}

	buf2.Reset()

	// ERROR: h1 handles and halts propagation (bubble = false)
	if err := logger.Error(context.Background(), "error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf1.String(), "error message") {
		t.Errorf("expected buf1 to contain error message, got: %s", buf1.String())
	}
	if buf2.Len() != 0 {
		t.Errorf("expected buf2 to be empty due to bubble=false on h1, got: %s", buf2.String())
	}
}

func TestToSlogLevel(t *testing.T) {
	tests := []struct {
		monogoLevel monogo.Level
		expected    slog.Level
	}{
		{monogo.DEBUG, slog.LevelDebug},
		{monogo.INFO, slog.LevelInfo},
		{monogo.NOTICE, slogadapter.LevelNotice},
		{monogo.WARNING, slog.LevelWarn},
		{monogo.ERROR, slog.LevelError},
		{monogo.CRITICAL, slogadapter.LevelCritical},
		{monogo.ALERT, slogadapter.LevelAlert},
		{monogo.EMERGENCY, slogadapter.LevelEmergency},
	}

	for _, tt := range tests {
		actual := slogadapter.ToSlogLevel(tt.monogoLevel)
		if actual != tt.expected {
			t.Errorf("ToSlogLevel(%v) = %v; want %v", tt.monogoLevel, actual, tt.expected)
		}
	}
}

func TestFromSlogLevel(t *testing.T) {
	tests := []struct {
		slogLevel slog.Level
		expected  monogo.Level
	}{
		{slog.LevelDebug, monogo.DEBUG},
		{slog.LevelInfo, monogo.INFO},
		{slogadapter.LevelNotice, monogo.NOTICE},
		{slog.LevelWarn, monogo.WARNING},
		{slog.LevelError, monogo.ERROR},
		{slogadapter.LevelCritical, monogo.CRITICAL},
		{slogadapter.LevelAlert, monogo.ALERT},
		{slogadapter.LevelEmergency, monogo.EMERGENCY},
	}

	for _, tt := range tests {
		actual := slogadapter.FromSlogLevel(tt.slogLevel)
		if actual != tt.expected {
			t.Errorf("FromSlogLevel(%v) = %v; want %v", tt.slogLevel, actual, tt.expected)
		}
	}
}

func TestReplaceLevelAttr(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected string
	}{
		{slogadapter.LevelNotice, "NOTICE"},
		{slogadapter.LevelCritical, "CRITICAL"},
		{slogadapter.LevelAlert, "ALERT"},
		{slogadapter.LevelEmergency, "EMERGENCY"},
	}

	for _, tt := range tests {
		attr := slog.Any(slog.LevelKey, tt.level)
		res := slogadapter.ReplaceLevelAttr(nil, attr)
		if res.Value.String() != tt.expected {
			t.Errorf("ReplaceLevelAttr(%v) = %s; want %s", tt.level, res.Value.String(), tt.expected)
		}
	}
}

func TestSlogHandlerCustomLevels(t *testing.T) {
	var buf bytes.Buffer
	slogH := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: slogadapter.ReplaceLevelAttr,
	})
	monoH := slogadapter.NewSlogHandler(slogH, monogo.DEBUG)
	logger := monogo.New("custom-chan", []monogo.Handler{monoH}, nil)

	// Notice
	if err := logger.Notice(context.Background(), "testing notice level"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `level=NOTICE`) {
		t.Errorf("expected level=NOTICE in output, got: %s", out)
	}
	if !strings.Contains(out, `severity=NOTICE`) {
		t.Errorf("expected severity=NOTICE in output, got: %s", out)
	}

	buf.Reset()

	// Critical
	if err := logger.Critical(context.Background(), "testing critical level"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, `level=CRITICAL`) {
		t.Errorf("expected level=CRITICAL in output, got: %s", out)
	}
	if !strings.Contains(out, `severity=CRITICAL`) {
		t.Errorf("expected severity=CRITICAL in output, got: %s", out)
	}
}

func TestSlogHandlerWithBufferFallback(t *testing.T) {
	var buf bytes.Buffer
	slogH := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	monoH := slogadapter.NewSlogHandler(slogH, monogo.DEBUG)
	bufH := handler.NewBuffer(monoH, 2, monogo.ERROR)

	_ = bufH.Handle(monogo.Record{Message: "buffered 1", Level: monogo.INFO})
	_ = bufH.Handle(monogo.Record{Message: "buffered 2", Level: monogo.INFO})

	out := buf.String()
	if !strings.Contains(out, `msg="buffered 1"`) {
		t.Errorf("expected buffered 1 in output, got: %s", out)
	}
	if !strings.Contains(out, `msg="buffered 2"`) {
		t.Errorf("expected buffered 2 in output, got: %s", out)
	}
}

var _ monogo.ProcessableHandler = (*slogadapter.SlogHandler)(nil)

func TestSlogHandlerWithProcessor(t *testing.T) {
	var buf bytes.Buffer
	slogH := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	monoH := slogadapter.NewSlogHandler(slogH, monogo.DEBUG,
		handler.WithProcessor(processor.Tag("slog_tag", "annotated")),
	)

	logger := monogo.New("slog-proc-test", []monogo.Handler{monoH}, nil)
	if err := logger.Info(context.Background(), "testing slog handler processor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `extra.slog_tag=annotated`) {
		t.Errorf("expected extra.slog_tag=annotated in slog output, got: %s", out)
	}
}
