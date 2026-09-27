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

func TestLineFormatterBatch(t *testing.T) {
	f := formatter.NewLine("", "")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "msg 1", Level: monogo.INFO, Channel: "app", Time: now},
		{Message: "msg 2", Level: monogo.ERROR, Channel: "app", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(bytes))
	}
	if !strings.Contains(lines[0], "app.INFO: msg 1") {
		t.Errorf("expected line 1 to contain info msg, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "app.ERROR: msg 2") {
		t.Errorf("expected line 2 to contain error msg, got: %s", lines[1])
	}
}

func TestJSONFormatterBatchNewlines(t *testing.T) {
	f := formatter.NewJSON("")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "batch 1", Level: monogo.INFO, Channel: "worker", Time: now},
		{Message: "batch 2", Level: monogo.WARNING, Channel: "worker", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var p1, p2 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &p1); err != nil {
		t.Fatalf("unmarshal line 1 error: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &p2); err != nil {
		t.Fatalf("unmarshal line 2 error: %v", err)
	}

	if p1["message"] != "batch 1" || p1["level_name"] != "INFO" {
		t.Errorf("unexpected payload 1: %v", p1)
	}
	if p2["message"] != "batch 2" || p2["level_name"] != "WARNING" {
		t.Errorf("unexpected payload 2: %v", p2)
	}
}

func TestJSONFormatterBatchJSON(t *testing.T) {
	f := formatter.NewJSON("").WithBatchMode(formatter.BatchModeJSON)
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "item 1", Level: monogo.DEBUG, Channel: "queue", Time: now},
		{Message: "item 2", Level: monogo.NOTICE, Channel: "queue", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("unmarshal JSON array error: %v (raw: %s)", err, string(bytes))
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 elements in JSON array, got %d", len(parsed))
	}
	if parsed[0]["message"] != "item 1" || parsed[0]["level_name"] != "DEBUG" {
		t.Errorf("unexpected first item: %v", parsed[0])
	}
	if parsed[1]["message"] != "item 2" || parsed[1]["level_name"] != "NOTICE" {
		t.Errorf("unexpected second item: %v", parsed[1])
	}
}
