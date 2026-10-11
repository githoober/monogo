package formatter_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

func TestLogstash_Basic(t *testing.T) {
	fmt := formatter.NewLogstash("my-app",
		formatter.WithLogstashSystemName("worker-01"),
		formatter.WithLogstashExtraKey("extra_fields"),
		formatter.WithLogstashContextKey("ctx_fields"),
	)

	rec := monogo.Record{
		Message: "order created",
		Level:   monogo.INFO,
		Channel: "billing",
		Time:    time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
		Context: map[string]interface{}{"order_id": 42},
		Extra:   map[string]interface{}{"worker_id": 7},
	}

	out, err := fmt.Format(rec)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v, out: %s", err, string(out))
	}

	if parsed["@version"] != float64(1) {
		t.Errorf("expected @version=1, got %v", parsed["@version"])
	}
	if parsed["type"] != "my-app" {
		t.Errorf("expected type 'my-app', got %v", parsed["type"])
	}
	if parsed["host"] != "worker-01" {
		t.Errorf("expected host 'worker-01', got %v", parsed["host"])
	}
	if parsed["channel"] != "billing" {
		t.Errorf("expected channel 'billing', got %v", parsed["channel"])
	}
	if parsed["level"] != "INFO" {
		t.Errorf("expected level 'INFO', got %v", parsed["level"])
	}
	if parsed["monolog_level"] != float64(200) {
		t.Errorf("expected monolog_level 200, got %v", parsed["monolog_level"])
	}

	ctxFields, ok := parsed["ctx_fields"].(map[string]interface{})
	if !ok || ctxFields["order_id"] != float64(42) {
		t.Errorf("expected ctx_fields.order_id 42, got %v", parsed["ctx_fields"])
	}

	extraFields, ok := parsed["extra_fields"].(map[string]interface{})
	if !ok || extraFields["worker_id"] != float64(7) {
		t.Errorf("expected extra_fields.worker_id 7, got %v", parsed["extra_fields"])
	}
}

func TestLogstash_Batch(t *testing.T) {
	fmt := formatter.NewLogstash("api")

	records := []monogo.Record{
		{Message: "msg 1", Level: monogo.DEBUG, Channel: "app", Time: time.Now().UTC()},
		{Message: "msg 2", Level: monogo.ERROR, Channel: "app", Time: time.Now().UTC()},
	}

	// 1. NDJSON mode
	ndjsonOut, err := fmt.FormatBatch(records)
	if err != nil {
		t.Fatalf("FormatBatch NDJSON failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(ndjsonOut)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// 2. JSON array mode
	fmtArray := formatter.NewLogstash("api",
		formatter.WithLogstashBatchMode(formatter.BatchModeJSON),
		formatter.WithLogstashDateFormat(time.RFC3339),
	)
	arrOut, err := fmtArray.FormatBatch(records)
	if err != nil {
		t.Fatalf("FormatBatch JSON array failed: %v", err)
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal(arrOut, &arr); err != nil {
		t.Fatalf("unmarshal array error: %v, out: %s", err, string(arrOut))
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 array entries, got %d", len(arr))
	}

	// Empty records batch returns empty
	emptyOut, err := fmt.FormatBatch(nil)
	if err != nil {
		t.Fatalf("empty batch failed: %v", err)
	}
	if len(emptyOut) != 0 {
		t.Errorf("expected empty output for empty batch")
	}
}
