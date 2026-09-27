package processor_test

import (
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

func TestProcessors(t *testing.T) {
	rec := monogo.Record{
		Message: "test",
		Level:   monogo.INFO,
	}

	// Test Tag
	tagProc := processor.Tag("env", "staging")
	rec = tagProc.Process(rec)
	if rec.Extra["env"] != "staging" {
		t.Errorf("Tag processor failed, got %v", rec.Extra["env"])
	}

	// Test Hostname
	hostProc := processor.Hostname()
	rec = hostProc.Process(rec)
	if _, ok := rec.Extra["hostname"].(string); !ok {
		t.Errorf("Hostname processor failed")
	}

	// Test Memory
	memProc := processor.Memory()
	rec = memProc.Process(rec)
	if mem, ok := rec.Extra["memory"].(map[string]interface{}); !ok || mem["alloc_bytes"] == nil {
		t.Errorf("Memory processor failed")
	}

	// Test UID
	uidProc := processor.UID()
	rec = uidProc.Process(rec)
	uid1, ok := rec.Extra["uid"].(string)
	if !ok || len(uid1) == 0 {
		t.Errorf("UID processor failed")
	}

	// Test Caller
	callerProc := processor.Caller(0)
	rec = callerProc.Process(rec)
	caller, ok := rec.Extra["caller"].(map[string]interface{})
	if !ok || caller["file"] == nil {
		t.Errorf("Caller processor failed")
	}
}
