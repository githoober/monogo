package processor_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/processor"
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

	// Test ProcessId
	pidProc := processor.ProcessId()
	rec = pidProc.Process(rec)
	pid, ok := rec.Extra["pid"].(int)
	if !ok || pid <= 0 {
		t.Errorf("ProcessId processor failed, got %v", rec.Extra["pid"])
	}

	// Test Process alias
	procAlias := processor.Process()
	recAlias := procAlias.Process(monogo.Record{Message: "alias"})
	if recAlias.Extra["pid"] != pid {
		t.Errorf("Process alias failed, got %v", recAlias.Extra["pid"])
	}
}

func TestGitProcessor(t *testing.T) {
	// Test Git with explicit config
	cfg := processor.GitConfig{
		Commit:   "a1b2c3d4e5",
		Branch:   "feature/test",
		Modified: true,
		Time:     "2026-10-03T12:00:00Z",
	}
	gitProc := processor.Git(cfg)
	rec := gitProc.Process(monogo.Record{Message: "git test"})

	gitMap, ok := rec.Extra["git"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['git'] to be a map, got: %v", rec.Extra["git"])
	}
	if gitMap["commit"] != "a1b2c3d4e5" {
		t.Errorf("expected commit 'a1b2c3d4e5', got: %v", gitMap["commit"])
	}
	if gitMap["branch"] != "feature/test" {
		t.Errorf("expected branch 'feature/test', got: %v", gitMap["branch"])
	}
	if gitMap["modified"] != true {
		t.Errorf("expected modified true, got: %v", gitMap["modified"])
	}
	if gitMap["time"] != "2026-10-03T12:00:00Z" {
		t.Errorf("expected time '2026-10-03T12:00:00Z', got: %v", gitMap["time"])
	}

	// Test Git auto-discovery fallback with env vars
	t.Setenv("GIT_COMMIT", "envcommit123")
	t.Setenv("GIT_BRANCH", "main")
	autoGitProc := processor.Git()
	rec2 := autoGitProc.Process(monogo.Record{Message: "auto git"})
	gitMap2, ok := rec2.Extra["git"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['git'] to be a map, got: %v", rec2.Extra["git"])
	}
	if gitMap2["commit"] == "" {
		t.Errorf("expected non-empty commit from env or build info")
	}
}

func TestEnvProcessors(t *testing.T) {
	t.Setenv("MONOGO_ENV_CLUSTER", "k8s-prod-1")
	t.Setenv("MONOGO_ENV_REGION", "us-east-1")
	t.Setenv("MONOGO_ENV_UNSET", "")

	// Test Env grouping
	envProc := processor.Env("MONOGO_ENV_CLUSTER", "MONOGO_ENV_REGION", "MONOGO_ENV_UNSET", "NON_EXISTENT_VAR")
	rec := envProc.Process(monogo.Record{Message: "env test"})

	envMap, ok := rec.Extra["env"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['env'] to be map, got: %v", rec.Extra["env"])
	}
	if envMap["MONOGO_ENV_CLUSTER"] != "k8s-prod-1" {
		t.Errorf("expected cluster 'k8s-prod-1', got: %v", envMap["MONOGO_ENV_CLUSTER"])
	}
	if envMap["MONOGO_ENV_REGION"] != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got: %v", envMap["MONOGO_ENV_REGION"])
	}
	if _, exists := envMap["NON_EXISTENT_VAR"]; exists {
		t.Errorf("non-existent var should not be present in env map")
	}

	// Test EnvMap top-level key mapping
	envMapProc := processor.EnvMap(map[string]string{
		"MONOGO_ENV_CLUSTER": "cluster",
		"MONOGO_ENV_REGION":  "region",
	})
	rec2 := envMapProc.Process(monogo.Record{Message: "env map test"})
	if rec2.Extra["cluster"] != "k8s-prod-1" {
		t.Errorf("expected extra['cluster']='k8s-prod-1', got: %v", rec2.Extra["cluster"])
	}
	if rec2.Extra["region"] != "us-east-1" {
		t.Errorf("expected extra['region']='us-east-1', got: %v", rec2.Extra["region"])
	}
}

func TestUIDProcessor(t *testing.T) {
	// Interface checks
	var _ monogo.Processor = (*processor.UIDProcessor)(nil)
	var _ monogo.Resettable = (*processor.UIDProcessor)(nil)

	// Default length (16)
	u := processor.NewUIDProcessor()
	uid1 := u.UID()
	if len(uid1) != 16 {
		t.Fatalf("expected default UID length 16, got %d (%s)", len(uid1), uid1)
	}

	// Stays constant across calls
	rec1 := u.Process(monogo.Record{Message: "m1"})
	rec2 := u.Process(monogo.Record{Message: "m2"})
	if rec1.Extra["uid"] != uid1 || rec2.Extra["uid"] != uid1 {
		t.Fatalf("expected UID to remain constant across Process calls")
	}

	// Reset regenerates UID
	if err := u.Reset(context.Background()); err != nil {
		t.Fatalf("unexpected error from u.Reset: %v", err)
	}
	uid2 := u.UID()
	if uid2 == uid1 {
		t.Fatalf("expected UID to change after Reset()")
	}
	if len(uid2) != 16 {
		t.Fatalf("expected UID length 16, got %d", len(uid2))
	}

	rec3 := u.Process(monogo.Record{Message: "m3"})
	if rec3.Extra["uid"] != uid2 {
		t.Fatalf("expected new UID after reset to be %s, got %v", uid2, rec3.Extra["uid"])
	}

	// Custom length (7 chars, matching PHP Monolog default)
	u7 := processor.UID(7)
	if len(u7.UID()) != 7 {
		t.Fatalf("expected UID length 7, got %d (%s)", len(u7.UID()), u7.UID())
	}

	// Custom length (32 chars)
	u32 := processor.NewUIDProcessor(32)
	if len(u32.UID()) != 32 {
		t.Fatalf("expected UID length 32, got %d (%s)", len(u32.UID()), u32.UID())
	}

	// Boundary length tests: 1 (MinUIDLength) and 64 (MaxUIDLength)
	uMin := processor.NewUIDProcessor(1)
	if len(uMin.UID()) != 1 {
		t.Fatalf("expected UID length 1, got %d", len(uMin.UID()))
	}
	uMax := processor.NewUIDProcessor(64)
	if len(uMax.UID()) != 64 {
		t.Fatalf("expected UID length 64, got %d", len(uMax.UID()))
	}

	// Out-of-bounds lengths should safely default to 16 without overflow, panic, or OOM
	uNegative := processor.NewUIDProcessor(-10)
	if len(uNegative.UID()) != 16 {
		t.Fatalf("expected negative length to default to 16, got %d", len(uNegative.UID()))
	}
	uZero := processor.NewUIDProcessor(0)
	if len(uZero.UID()) != 16 {
		t.Fatalf("expected zero length to default to 16, got %d", len(uZero.UID()))
	}
	uOverMax := processor.NewUIDProcessor(65)
	if len(uOverMax.UID()) != 16 {
		t.Fatalf("expected length > 64 to default to 16, got %d", len(uOverMax.UID()))
	}
	uHuge := processor.NewUIDProcessor(math.MaxInt)
	if len(uHuge.UID()) != 16 {
		t.Fatalf("expected math.MaxInt to default to 16 without overflow/panic, got %d", len(uHuge.UID()))
	}

	// Concurrent usage safety
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = u.Process(monogo.Record{Message: "concurrent"})
			}
			done <- true
		}()
		go func() {
			for j := 0; j < 20; j++ {
				_ = u.Reset(context.Background())
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

type failingReader struct{}

func (f *failingReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("entropy failure")
}

func TestUIDProcessor_FallbackEntropyFailure(t *testing.T) {
	restore := processor.SetRandReaderForTest(&failingReader{})
	defer restore()

	lengths := []int{1, 2, 7, 16, 32, 64}
	for _, l := range lengths {
		u := processor.NewUIDProcessor(l)
		uid1 := u.UID()
		if len(uid1) != l {
			t.Fatalf("expected fallback UID length %d, got %d (%s)", l, len(uid1), uid1)
		}
		// Reset immediately (same clock tick possible)
		if err := u.Reset(context.Background()); err != nil {
			t.Fatalf("unexpected reset error: %v", err)
		}
		uid2 := u.UID()
		if len(uid2) != l {
			t.Fatalf("expected fallback UID length %d after reset, got %d (%s)", l, len(uid2), uid2)
		}
		if uid1 == uid2 {
			t.Fatalf("expected fallback UIDs of length %d to be unique across calls, but got identical: %s", l, uid1)
		}
	}
}


