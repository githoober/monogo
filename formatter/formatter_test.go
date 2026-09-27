package formatter_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

func TestLineFormatter(t *testing.T) {
	f := formatter.NewLine("", "")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "test message",
		Level:   monogo.INFO,
		Channel: "main",
		Time:    now,
		Context: map[string]interface{}{"user_id": 42},
		Extra:   map[string]interface{}{"ip": "127.0.0.1"},
	}

	res, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	str := string(res)
	if !strings.Contains(str, "2025-01-01T12:00:00Z") {
		t.Errorf("expected timestamp in line format output, got: %s", str)
	}
	if !strings.Contains(str, "main.INFO: test message") {
		t.Errorf("expected channel/level/message, got: %s", str)
	}
	if !strings.Contains(str, `"user_id":42`) {
		t.Errorf("expected context JSON, got: %s", str)
	}
	if !strings.Contains(str, `"ip":"127.0.0.1"`) {
		t.Errorf("expected extra JSON, got: %s", str)
	}
}

func TestJSONFormatter(t *testing.T) {
	f := formatter.NewJSON("")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "json test message",
		Level:   monogo.ERROR,
		Channel: "api",
		Time:    now,
		Context: map[string]interface{}{"error": "db timeout"},
	}

	res, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(res, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if parsed["message"] != "json test message" {
		t.Errorf("unexpected message: %v", parsed["message"])
	}
	if parsed["level_name"] != "ERROR" {
		t.Errorf("unexpected level_name: %v", parsed["level_name"])
	}
	if parsed["channel"] != "api" {
		t.Errorf("unexpected channel: %v", parsed["channel"])
	}
	ctx, ok := parsed["context"].(map[string]interface{})
	if !ok || ctx["error"] != "db timeout" {
		t.Errorf("unexpected context: %v", parsed["context"])
	}
}
