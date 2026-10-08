package handler_test

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func TestRotatingJSONFile_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")

	h := handler.NewRotatingJSONFile(logPath, monogo.DEBUG)
	ctx := t.Context()

	rec := monogo.Record{
		Message: "order created",
		Level:   monogo.INFO,
		Channel: "billing",
		Time:    time.Now().UTC(),
		Context: map[string]interface{}{"order_id": 42},
	}

	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v, data: %s", err, string(data))
	}

	if parsed["message"] != "order created" {
		t.Errorf("expected message 'order created', got %v", parsed["message"])
	}
	if parsed["channel"] != "billing" {
		t.Errorf("expected channel 'billing', got %v", parsed["channel"])
	}
	if parsed["level_name"] != "INFO" {
		t.Errorf("expected level_name 'INFO', got %v", parsed["level_name"])
	}
	if parsed["level"] != float64(200) {
		t.Errorf("expected level 200, got %v", parsed["level"])
	}
}

func TestRotatingJSONFile_RotationAndBackups(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "rotate.log")

	// Set small maxSize so each record triggers rotation
	h := handler.NewRotatingJSONFile(logPath, monogo.DEBUG,
		handler.WithMaxSize(80),
		handler.WithMaxBackups(2),
	)
	ctx := t.Context()

	// Write 4 records
	for i := 1; i <= 4; i++ {
		rec := monogo.Record{
			Message: "test message number",
			Level:   monogo.INFO,
			Channel: "test",
			Time:    time.Now().UTC(),
			Context: map[string]interface{}{"i": i},
		}
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("Handle %d failed: %v", i, err)
		}
	}

	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Check active file exists
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("active file missing: %v", err)
	}

	// Backup 1 and 2 should exist
	backup1 := filepath.Join(tmpDir, "rotate.1.log")
	backup2 := filepath.Join(tmpDir, "rotate.2.log")
	backup3 := filepath.Join(tmpDir, "rotate.3.log")

	if _, err := os.Stat(backup1); err != nil {
		t.Errorf("expected backup 1 to exist: %v", err)
	}
	if _, err := os.Stat(backup2); err != nil {
		t.Errorf("expected backup 2 to exist: %v", err)
	}
	// Backup 3 should have been purged (maxBackups = 2)
	if _, err := os.Stat(backup3); !os.IsNotExist(err) {
		t.Errorf("expected backup 3 to be purged, but it exists")
	}

	// Verify backup1 contains valid JSON
	b1Data, err := os.ReadFile(backup1)
	if err != nil {
		t.Fatalf("failed to read backup1: %v", err)
	}
	var b1JSON map[string]interface{}
	if err := json.Unmarshal(b1Data, &b1JSON); err != nil {
		t.Fatalf("backup1 is not valid JSON: %v", err)
	}
}

func TestRotatingJSONFile_Compression(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "compress.log")

	h := handler.NewJSONRotatingFile(logPath, monogo.DEBUG,
		handler.WithMaxSize(80),
		handler.WithMaxBackups(2),
		handler.WithCompress(true),
	)
	ctx := t.Context()

	// Write records to trigger rotation with compression
	for i := 1; i <= 3; i++ {
		rec := monogo.Record{
			Message: "compression test log message",
			Level:   monogo.INFO,
			Channel: "test",
			Time:    time.Now().UTC(),
			Context: map[string]interface{}{"idx": i},
		}
		if err := h.Handle(ctx, rec); err != nil {
			t.Fatalf("Handle %d failed: %v", i, err)
		}
	}

	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Backup 1 should be a .gz file
	gzBackup := filepath.Join(tmpDir, "compress.1.log.gz")
	f, err := os.Open(gzBackup)
	if err != nil {
		t.Fatalf("expected compressed backup %s to exist: %v", gzBackup, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	defer gzr.Close()

	decompressed, err := io.ReadAll(gzr)
	if err != nil {
		t.Fatalf("read decompressed failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(decompressed, &parsed); err != nil {
		t.Fatalf("decompressed data is not valid JSON: %v", err)
	}
	if parsed["message"] != "compression test log message" {
		t.Errorf("unexpected message in gzip: %v", parsed["message"])
	}
}

func TestRotatingJSONFile_BatchHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "batch.log")

	h := handler.NewRotatingJSONFile(logPath, monogo.INFO)
	ctx := t.Context()

	records := []monogo.Record{
		{Message: "batch item 1", Level: monogo.INFO, Channel: "batch", Time: time.Now().UTC()},
		{Message: "batch item 2", Level: monogo.ERROR, Channel: "batch", Time: time.Now().UTC()},
	}

	if err := h.HandleBatch(ctx, records); err != nil {
		t.Fatalf("HandleBatch failed: %v", err)
	}

	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("expected non-empty log output")
	}
}

func TestRotatingFile_StandardLineFormatter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "standard.log")

	h := handler.NewRotatingFile(logPath, monogo.DEBUG,
		handler.WithMaxSizeMB(10),
		handler.WithMaxBackups(3),
	)
	ctx := t.Context()

	rec := monogo.Record{
		Message: "standard line log",
		Level:   monogo.WARNING,
		Channel: "app",
		Time:    time.Now().UTC(),
	}

	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if h.Writer() == nil {
		t.Errorf("expected non-nil Writer()")
	}
	if h.Writer().Filename() != logPath {
		t.Errorf("expected filename %s, got %s", logPath, h.Writer().Filename())
	}

	if err := h.Sync(); err != nil {
		t.Errorf("Sync failed: %v", err)
	}

	if err := h.Reset(ctx); err != nil {
		t.Errorf("Reset failed: %v", err)
	}

	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("expected non-empty output")
	}
}

func TestRotatingFile_MaxAgeCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "age.log")

	// Pre-create old backup files with timestamp 5 days ago
	oldBackupAncient := filepath.Join(tmpDir, "age.ancient.log")
	if err := os.WriteFile(oldBackupAncient, []byte("ancient log\n"), 0644); err != nil {
		t.Fatalf("failed to create old backup: %v", err)
	}
	oldBackupNumeric := filepath.Join(tmpDir, "age.8.log")
	if err := os.WriteFile(oldBackupNumeric, []byte("ancient numeric log\n"), 0644); err != nil {
		t.Fatalf("failed to create old backup: %v", err)
	}
	oldTime := time.Now().Add(-5 * 24 * time.Hour)
	if err := os.Chtimes(oldBackupAncient, oldTime, oldTime); err != nil {
		t.Fatalf("failed to chtimes ancient: %v", err)
	}
	if err := os.Chtimes(oldBackupNumeric, oldTime, oldTime); err != nil {
		t.Fatalf("failed to chtimes numeric: %v", err)
	}

	h := handler.NewRotatingFile(logPath, monogo.DEBUG,
		handler.WithMaxSize(50),
		handler.WithMaxAge(2), // Max age 2 days
		handler.WithMaxAgeDays(2),
	)
	ctx := t.Context()

	// Write to trigger rotation and cleanup
	for i := 1; i <= 3; i++ {
		_ = h.Handle(ctx, monogo.Record{
			Message: "trigger rotation and cleanup",
			Level:   monogo.INFO,
			Time:    time.Now().UTC(),
		})
	}
	_ = h.Close(ctx)

	// Old backups should have been removed
	if _, err := os.Stat(oldBackupAncient); !os.IsNotExist(err) {
		t.Errorf("expected oldBackupAncient to be removed by maxAgeDays cleanup")
	}
	if _, err := os.Stat(oldBackupNumeric); !os.IsNotExist(err) {
		t.Errorf("expected oldBackupNumeric to be removed by maxAgeDays cleanup")
	}
}

func TestRotatingFile_ManualRotate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "manual.log")

	h := handler.NewRotatingFile(logPath, monogo.DEBUG)
	ctx := t.Context()

	_ = h.Handle(ctx, monogo.Record{
		Message: "before rotate",
		Level:   monogo.INFO,
		Time:    time.Now().UTC(),
	})

	if err := h.Rotate(); err != nil {
		t.Fatalf("manual Rotate failed: %v", err)
	}

	backup1 := filepath.Join(tmpDir, "manual.1.log")
	if _, err := os.Stat(backup1); err != nil {
		t.Errorf("expected backup 1 to exist after manual rotate: %v", err)
	}

	_ = h.Close(ctx)
}

func TestRotatingFile_DailyRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daily.log")

	h := handler.NewRotatingFile(logPath, monogo.DEBUG,
		handler.WithDailyRotation(true),
	)
	ctx := t.Context()

	// First write opens the file
	_ = h.Handle(ctx, monogo.Record{
		Message: "day 1 message",
		Level:   monogo.INFO,
		Time:    time.Now().UTC(),
	})

	_ = h.Close(ctx)
}

func TestRotatingJSONFile_WithProcessorAndBubbling(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "proc.log")

	h := handler.NewRotatingJSONFile(logPath, monogo.DEBUG,
		handler.WithBubble(false),
		handler.WithProcessor(processor.ProcessId()),
	)
	ctx := t.Context()

	if h.Bubble() {
		t.Errorf("expected Bubble() to be false")
	}

	rec := monogo.Record{
		Message: "processed json log",
		Level:   monogo.INFO,
		Channel: "core",
		Time:    time.Now().UTC(),
	}

	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	_ = h.Close(ctx)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	extra, ok := parsed["extra"].(map[string]interface{})
	if !ok || extra["pid"] == nil {
		t.Errorf("expected extra.pid to be populated by processor, got: %v", parsed["extra"])
	}
}
