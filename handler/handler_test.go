package handler_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func TestStreamHandler(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.INFO)

	if !sh.IsHandling(monogo.INFO) {
		t.Errorf("Stream handler should handle INFO")
	}
	if sh.IsHandling(monogo.DEBUG) {
		t.Errorf("Stream handler should not handle DEBUG")
	}

	rec := monogo.Record{
		Message: "stream test",
		Level:   monogo.INFO,
		Channel: "app",
	}

	if err := sh.Handle(rec); err != nil {
		t.Fatalf("Stream handle error: %v", err)
	}

	if !strings.Contains(buf.String(), "app.INFO: stream test") {
		t.Errorf("Unexpected stream output: %s", buf.String())
	}
}

func TestStreamHandlerJSONFile(t *testing.T) {
	var buf bytes.Buffer
	sh := handler.NewStream(&buf, monogo.DEBUG, handler.WithFormatter(formatter.NewJSON("")))

	logger := monogo.New("json-file-app", []monogo.Handler{sh}, nil)

	err := logger.Info("writing json logs", map[string]interface{}{"file": "app.log", "status": "ok"})
	if err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid json output: %v (raw: %s)", err, buf.String())
	}

	if parsed["channel"] != "json-file-app" {
		t.Errorf("expected channel json-file-app, got %v", parsed["channel"])
	}
	if parsed["message"] != "writing json logs" {
		t.Errorf("expected message 'writing json logs', got %v", parsed["message"])
	}

	ctx, ok := parsed["context"].(map[string]interface{})
	if !ok || ctx["file"] != "app.log" {
		t.Errorf("expected context file=app.log, got %v", parsed["context"])
	}
}

func TestRotatingFileHandler(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotating.log")

	rotH := handler.NewRotatingFile(logPath, monogo.INFO,
		handler.WithMaxSize(1),
		handler.WithMaxBackups(2),
		handler.WithMaxAge(7),
	)
	defer rotH.Close()

	if !rotH.Bubble() {
		t.Errorf("expected default Bubble to be true")
	}

	logger := monogo.New("rot-app", []monogo.Handler{rotH}, nil)

	err := logger.Info("rotating file log message", map[string]interface{}{"test": "rotation"})
	if err != nil {
		t.Fatalf("unexpected error logging to rotating file: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read rotated log file: %v", err)
	}

	if !strings.Contains(string(content), "rot-app.INFO: rotating file log message") {
		t.Errorf("rotated log file missing expected content, got: %s", string(content))
	}
}

func TestRotatingFileHandlerBubbling(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotating_bubble.log")

	rotH := handler.NewRotatingFile(logPath, monogo.ERROR,
		handler.WithBubble(false),
		handler.WithRotation(handler.RotatingFileOptions{
			MaxSizeMB:  1,
			MaxBackups: 2,
		}),
	)
	defer rotH.Close()

	if rotH.Bubble() {
		t.Errorf("expected Bubble to be false with WithBubble(false)")
	}
}

func TestFingersCrossedHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	fc := handler.NewFingersCrossed(testH, monogo.ERROR, 10)

	_ = fc.Handle(monogo.Record{Message: "debug 1", Level: monogo.DEBUG})
	_ = fc.Handle(monogo.Record{Message: "info 1", Level: monogo.INFO})

	if len(testH.Records()) != 0 {
		t.Fatalf("FingersCrossed should not have flushed records yet")
	}

	_ = fc.Handle(monogo.Record{Message: "error 1", Level: monogo.ERROR})

	recs := testH.Records()
	if len(recs) != 3 {
		t.Fatalf("Expected 3 records after error trigger, got %d", len(recs))
	}
	if recs[0].Message != "debug 1" || recs[1].Message != "info 1" || recs[2].Message != "error 1" {
		t.Errorf("Unexpected flushed records order/content: %v", recs)
	}

	_ = fc.Handle(monogo.Record{Message: "debug 2 post-trigger", Level: monogo.DEBUG})
	if len(testH.Records()) != 4 {
		t.Errorf("Expected 4 records after post-trigger log, got %d", len(testH.Records()))
	}
}

func TestFilterHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	filterH := handler.NewFilter(testH, monogo.INFO, monogo.ERROR)

	filterH.Handle(monogo.Record{Message: "debug", Level: monogo.DEBUG})
	filterH.Handle(monogo.Record{Message: "info", Level: monogo.INFO})
	filterH.Handle(monogo.Record{Message: "crit", Level: monogo.CRITICAL})

	recs := testH.Records()
	if len(recs) != 1 || recs[0].Message != "info" {
		t.Errorf("Filter handler failed, expected only 'info', got: %v", recs)
	}
}

func TestGroupHandler(t *testing.T) {
	t1 := handler.NewTest(monogo.DEBUG)
	t2 := handler.NewTest(monogo.WARNING)
	group := handler.NewGroup([]monogo.Handler{t1, t2})

	group.Handle(monogo.Record{Message: "info msg", Level: monogo.INFO})
	group.Handle(monogo.Record{Message: "warn msg", Level: monogo.WARNING})

	if len(t1.Records()) != 2 {
		t.Errorf("t1 should have 2 records, got %d", len(t1.Records()))
	}
	if len(t2.Records()) != 1 {
		t.Errorf("t2 should have 1 record, got %d", len(t2.Records()))
	}
}

func TestBufferHandler(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	bufH := handler.NewBuffer(testH, 3, monogo.ERROR)

	bufH.Handle(monogo.Record{Message: "msg 1", Level: monogo.INFO})
	bufH.Handle(monogo.Record{Message: "msg 2", Level: monogo.INFO})

	if len(testH.Records()) != 0 {
		t.Errorf("buffer should not have flushed yet")
	}

	bufH.Handle(monogo.Record{Message: "msg 3 error", Level: monogo.ERROR})
	if len(testH.Records()) != 3 {
		t.Errorf("buffer should have flushed 3 records, got %d", len(testH.Records()))
	}
}

func TestNullAndTestHandler(t *testing.T) {
	nullH := handler.NewNull()
	if err := nullH.Handle(monogo.Record{Message: "test", Level: monogo.DEBUG}); err != nil {
		t.Errorf("null handler handle error: %v", err)
	}

	testH := handler.NewTest(monogo.DEBUG)
	testH.Handle(monogo.Record{Message: "find me", Level: monogo.INFO})

	found := testH.HasRecord(func(r monogo.Record) bool {
		return r.Message == "find me"
	})
	if !found {
		t.Errorf("Test handler HasRecord failed to find record")
	}
}

func TestBaseHandlerBubble(t *testing.T) {
	// Default bubbling (no options)
	bhDefault := handler.NewBaseHandler(monogo.INFO)
	if !bhDefault.Bubble() {
		t.Errorf("expected default bubble to be true")
	}

	// Explicit bubbling = false using WithBubble option
	bhNoBubble := handler.NewBaseHandler(monogo.INFO, handler.WithBubble(false))
	if bhNoBubble.Bubble() {
		t.Errorf("expected bubble to be false when configured with WithBubble(false)")
	}
}

func TestStreamHandlerBubbling(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	// sh1 configured with bubble = false using WithBubble option
	sh1 := handler.NewStream(&buf1, monogo.ERROR, handler.WithBubble(false))
	sh2 := handler.NewStream(&buf2, monogo.DEBUG)

	logger := monogo.New("bubble-stream-test", []monogo.Handler{sh1, sh2}, nil)

	// INFO: sh1 does not handle, so sh2 receives it
	if err := logger.Info("info msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf1.Len() != 0 {
		t.Errorf("expected buf1 to be empty, got: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "info msg") {
		t.Errorf("expected buf2 to contain info msg, got: %s", buf2.String())
	}

	buf2.Reset()

	// ERROR: sh1 handles it and stops bubbling (sh1.Bubble() == false), sh2 should not receive it
	if err := logger.Error("error msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf1.String(), "error msg") {
		t.Errorf("expected buf1 to contain error msg, got: %s", buf1.String())
	}
	if buf2.Len() != 0 {
		t.Errorf("expected buf2 to be empty due to bubble=false on sh1, got: %s", buf2.String())
	}
}

