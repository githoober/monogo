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
)

func TestSlogHandler(t *testing.T) {
	var buf bytes.Buffer
	slogH := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	monoH := slogadapter.NewSlogHandler(slogH, monogo.DEBUG)

	logger := monogo.New("test-channel", []monogo.Handler{monoH}, nil)

	err := logger.Info("hello slog backend", map[string]interface{}{"user": "alice"})
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
	if err := logger.Info("info message"); err != nil {
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
	if err := logger.Error("error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf1.String(), "error message") {
		t.Errorf("expected buf1 to contain error message, got: %s", buf1.String())
	}
	if buf2.Len() != 0 {
		t.Errorf("expected buf2 to be empty due to bubble=false on h1, got: %s", buf2.String())
	}
}

