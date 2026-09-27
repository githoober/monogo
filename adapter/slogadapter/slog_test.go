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
	monoH := slogadapter.NewSlogHandler(slogH, monolog.DEBUG)

	logger := monolog.New("test-channel", []monolog.Handler{monoH}, nil)

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
	testH := handler.NewTest(monolog.DEBUG)
	monoLogger := monolog.New("app-channel", []monolog.Handler{testH}, nil)

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
	if rec.Level != monolog.INFO {
		t.Errorf("unexpected level: %v", rec.Level)
	}
	if rec.Context["key"] != "val" {
		t.Errorf("unexpected context val: %v", rec.Context["key"])
	}
}
