package zerologadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/zerologadapter"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
	"github.com/rs/zerolog"
)

func TestZerologHandler(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Timestamp().Logger()

	zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG)
	logger := monogo.New("zerolog-chan", []monogo.Handler{zh}, nil)

	err := logger.Error(context.Background(), "something failed", map[string]interface{}{"retry_count": 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal zerolog output: %v (raw: %s)", err, buf.String())
	}

	if res["level"] != "error" {
		t.Errorf("expected level error, got %v", res["level"])
	}

	if res["message"] != "something failed" {
		t.Errorf("expected message 'something failed', got %v", res["message"])
	}

	if res["channel"] != "zerolog-chan" {
		t.Errorf("expected channel 'zerolog-chan', got %v", res["channel"])
	}

	if res["retry_count"] != float64(3) {
		t.Errorf("expected retry_count 3, got %v", res["retry_count"])
	}
}

func TestZerologHandlerBubbling(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	zLogger1 := zerolog.New(&buf1).With().Logger()
	zLogger2 := zerolog.New(&buf2).With().Logger()

	h1 := zerologadapter.NewZerologHandler(zLogger1, monogo.ERROR, handler.WithBubble(false))
	h2 := zerologadapter.NewZerologHandler(zLogger2, monogo.DEBUG)

	if !h2.Bubble() {
		t.Errorf("expected h2 default bubble to be true")
	}
	if h1.Bubble() {
		t.Errorf("expected h1 bubble to be false with WithBubble(false)")
	}

	logger := monogo.New("zerolog-bubble-test", []monogo.Handler{h1, h2}, nil)

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

func TestToZerologLevel(t *testing.T) {
	tests := []struct {
		monogoLevel monogo.Level
		expected    zerolog.Level
	}{
		{monogo.DEBUG, zerolog.DebugLevel},
		{monogo.INFO, zerolog.InfoLevel},
		{monogo.NOTICE, zerolog.InfoLevel},
		{monogo.WARNING, zerolog.WarnLevel},
		{monogo.ERROR, zerolog.ErrorLevel},
		{monogo.CRITICAL, zerolog.ErrorLevel},
		{monogo.ALERT, zerolog.ErrorLevel},
		{monogo.EMERGENCY, zerolog.ErrorLevel},
	}

	for _, tt := range tests {
		actual := zerologadapter.ToZerologLevel(tt.monogoLevel)
		if actual != tt.expected {
			t.Errorf("ToZerologLevel(%v) = %v; want %v", tt.monogoLevel, actual, tt.expected)
		}
	}
}

func TestFromZerologLevel(t *testing.T) {
	tests := []struct {
		zerologLevel zerolog.Level
		expected     monogo.Level
	}{
		{zerolog.TraceLevel, monogo.DEBUG},
		{zerolog.DebugLevel, monogo.DEBUG},
		{zerolog.InfoLevel, monogo.INFO},
		{zerolog.WarnLevel, monogo.WARNING},
		{zerolog.ErrorLevel, monogo.ERROR},
		{zerolog.FatalLevel, monogo.EMERGENCY},
		{zerolog.PanicLevel, monogo.ALERT},
		{zerolog.NoLevel, monogo.INFO},
	}

	for _, tt := range tests {
		actual := zerologadapter.FromZerologLevel(tt.zerologLevel)
		if actual != tt.expected {
			t.Errorf("FromZerologLevel(%v) = %v; want %v", tt.zerologLevel, actual, tt.expected)
		}
	}
}

func TestZerologHandlerSeverityAndNoFatal(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Logger()

	zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG)
	logger := monogo.New("zerolog-severity-test", []monogo.Handler{zh}, nil)

	// Notice: should map to "info" with severity "NOTICE"
	if err := logger.Notice(context.Background(), "notice message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res["level"] != "info" {
		t.Errorf("expected level 'info', got: %v", res["level"])
	}
	if res["severity"] != "NOTICE" {
		t.Errorf("expected severity 'NOTICE', got: %v", res["severity"])
	}

	buf.Reset()

	// Critical: must NOT exit, should map to "error" with severity "CRITICAL"
	if err := logger.Critical(context.Background(), "critical message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res = nil
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res["level"] != "error" {
		t.Errorf("expected level 'error', got: %v", res["level"])
	}
	if res["severity"] != "CRITICAL" {
		t.Errorf("expected severity 'CRITICAL', got: %v", res["severity"])
	}

	buf.Reset()

	// Regular Error: should NOT have extra "severity" field
	if err := logger.Error(context.Background(), "standard error message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res = nil
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if res["level"] != "error" {
		t.Errorf("expected level 'error', got: %v", res["level"])
	}
	if _, ok := res["severity"]; ok {
		t.Errorf("did not expect 'severity' field for standard ERROR, got: %v", res["severity"])
	}
}

func TestZerologHandlerWithBufferFallback(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Logger()
	zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG)
	bufH := handler.NewBuffer(zh, 2, monogo.ERROR)

	_ = bufH.Handle(monogo.Record{Message: "zero batch 1", Level: monogo.INFO, Channel: "zero-chan"})
	_ = bufH.Handle(monogo.Record{Message: "zero batch 2", Level: monogo.WARNING, Channel: "zero-chan"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d: %q", len(lines), buf.String())
	}
}

var _ monogo.ProcessableHandler = (*zerologadapter.ZerologHandler)(nil)

func TestZerologHandlerWithProcessor(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Logger()
	zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG,
		handler.WithProcessor(processor.Tag("cluster", "zerolog_us_east")),
	)

	logger := monogo.New("zerolog-proc-test", []monogo.Handler{zh}, nil)
	if err := logger.Info(context.Background(), "testing zerolog handler processor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	extra, ok := res["extra"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extra in zerolog output, got: %v", res["extra"])
	}
	if extra["cluster"] != "zerolog_us_east" {
		t.Errorf("expected extra.cluster = 'zerolog_us_east', got: %v", extra["cluster"])
	}
}



