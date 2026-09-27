package zerologadapter_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/zerologadapter"
	"github.com/rs/zerolog"
)

func TestZerologHandler(t *testing.T) {
	var buf bytes.Buffer
	zLogger := zerolog.New(&buf).With().Timestamp().Logger()

	zh := zerologadapter.NewZerologHandler(zLogger, monolog.DEBUG)
	logger := monolog.New("zerolog-chan", []monolog.Handler{zh}, nil)

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
