package zerologadapter_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/zerologadapter"
	"github.com/githoober/monogo/handler"
	"github.com/rs/zerolog"
)

func TestZerologHandler(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Timestamp().Logger()

	zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG)
	logger := monogo.New("zerolog-chan", []monogo.Handler{zh}, nil)

	err := logger.Error("something failed", map[string]interface{}{"retry_count": 3})
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

