package processor_test

import (
	"os"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

func TestProcessId(t *testing.T) {
	rec := monogo.Record{
		Message: "test",
		Level:   monogo.INFO,
	}

	pidProc := processor.ProcessId()
	rec = pidProc.Process(rec)
	pid, ok := rec.Extra["pid"].(int)
	if !ok || pid <= 0 {
		t.Fatalf("ProcessId processor failed, got %v", rec.Extra["pid"])
	}
	if pid != os.Getpid() {
		t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
	}

	// Test Process alias
	procAlias := processor.Process()
	rec2 := procAlias.Process(monogo.Record{Message: "test2"})
	if rec2.Extra["pid"] != os.Getpid() {
		t.Errorf("Process alias failed, got %v", rec2.Extra["pid"])
	}
}
