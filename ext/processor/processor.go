package processor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/githoober/monogo"
	coreprocessor "github.com/githoober/monogo/processor"
)

// Caller adds file, line, and function name information to Extra["caller"].
func Caller(skipFrames int) monogo.ProcessorFunc {
	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		// Skip runtime.Callers + this wrapper frame + requested skipFrames
		pc, file, line, ok := runtime.Caller(skipFrames + 3)
		if ok {
			fn := runtime.FuncForPC(pc)
			fnName := ""
			if fn != nil {
				fnName = fn.Name()
			}
			r.Extra["caller"] = map[string]interface{}{
				"file":     file,
				"line":     line,
				"function": fnName,
			}
		}
		return r
	}
}

// Hostname adds the OS hostname to Extra["hostname"].
func Hostname() monogo.ProcessorFunc {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["hostname"] = host
		return r
	}
}

// Memory adds memory allocation stats (Alloc and TotalAlloc) to Extra["memory"].
func Memory() monogo.ProcessorFunc {
	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		r.Extra["memory"] = map[string]interface{}{
			"alloc_bytes":       m.Alloc,
			"total_alloc_bytes": m.TotalAlloc,
			"sys_bytes":         m.Sys,
		}
		return r
	}
}

// Tag adds fixed key/value tag to Extra.
func Tag(key string, value interface{}) monogo.ProcessorFunc {
	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra[key] = value
		return r
	}
}

// UIDProcessor generates and attaches a unique identifier string to Extra["uid"].
// It maintains the same UID across log records within a cycle and regenerates a new UID when Reset() is called.
type UIDProcessor struct {
	mu     sync.RWMutex
	length int
	uid    string
}

// Compile-time interface assertions.
var (
	_ monogo.Processor  = (*UIDProcessor)(nil)
	_ monogo.Resettable = (*UIDProcessor)(nil)
)

const (
	// DefaultUIDLength is the default character length of generated UIDs (16 hex characters).
	DefaultUIDLength = 16
	// MinUIDLength is the minimum supported character length of a UID.
	MinUIDLength = 1
	// MaxUIDLength is the maximum supported character length of a UID (matching Monolog's 64-character limit).
	MaxUIDLength = 64
)

// NewUIDProcessor creates a new resettable UIDProcessor with an optional hex length (default: 16 characters).
// Length must be between 1 and 64 characters (inclusive), matching Monolog's UidProcessor constraints.
// If omitted or outside this range, DefaultUIDLength (16) is used.
// The UID remains constant across log records until Reset() is called, making it ideal for tracking
// requests, jobs, or operations across a lifecycle in long-running processes (workers, servers).
func NewUIDProcessor(length ...int) *UIDProcessor {
	l := DefaultUIDLength
	if len(length) > 0 {
		if length[0] >= MinUIDLength && length[0] <= MaxUIDLength {
			l = length[0]
		}
	}
	p := &UIDProcessor{
		length: l,
		uid:    generateUID(l),
	}
	return p
}

// UID returns the current UID string.
func (u *UIDProcessor) UID() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.uid
}

// Reset generates a new unique identifier.
func (u *UIDProcessor) Reset(_ context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.uid = generateUID(u.length)
	return nil
}

// Process enriches the log record by adding the current UID to Extra["uid"].
func (u *UIDProcessor) Process(r monogo.Record) monogo.Record {
	u.mu.RLock()
	uid := u.uid
	u.mu.RUnlock()

	if r.Extra == nil {
		r.Extra = make(map[string]interface{})
	}
	r.Extra["uid"] = uid
	return r
}

var (
	randReader         io.Reader = rand.Reader
	uidFallbackMu      sync.Mutex
	uidFallbackCounter uint64
)

func fallbackUID(length int) string {
	uidFallbackMu.Lock()
	uidFallbackCounter++
	cnt := uidFallbackCounter
	uidFallbackMu.Unlock()

	timeHex := fmt.Sprintf("%016x", time.Now().UnixNano())
	cntHex := fmt.Sprintf("%016x", cnt)

	if length <= 32 {
		// Retain the low-order counter portion so it contributes to all supported truncated identifiers (1..32),
		// ensuring uniqueness even if multiple calls occur within the same nanosecond clock tick.
		cntLen := (length + 1) / 2
		timeLen := length - cntLen
		return timeHex[len(timeHex)-timeLen:] + cntHex[len(cntHex)-cntLen:]
	}

	h := timeHex + cntHex
	for len(h) < length {
		h += "0"
	}
	return h[:length]
}

func generateUID(length int) string {
	if length < MinUIDLength || length > MaxUIDLength {
		length = DefaultUIDLength
	}
	bytesLen := (length + 1) / 2
	b := make([]byte, bytesLen)
	if _, err := randReader.Read(b); err != nil {
		return fallbackUID(length)
	}

	h := hex.EncodeToString(b)
	if len(h) > length {
		return h[:length]
	}
	return h
}

// UID creates a new resettable UIDProcessor (default: 16 characters).
// It maintains the same UID across log records and regenerates a new UID when Reset() is called.
func UID(length ...int) *UIDProcessor {
	return NewUIDProcessor(length...)
}

// ProcessId adds the operating system process ID (os.Getpid()) to Extra["pid"].
func ProcessId() monogo.ProcessorFunc {
	return coreprocessor.ProcessId()
}

// Process is an alias for ProcessId.
func Process() monogo.ProcessorFunc {
	return coreprocessor.Process()
}

// GitConfig holds VCS details to inject into log records.
type GitConfig struct {
	Commit   string
	Branch   string
	Modified bool
	Time     string
}

// Git adds Git VCS metadata (commit, branch, dirty state) to Extra["git"].
// If configs are omitted, it automatically discovers commit and revision details
// from Go's build info (runtime/debug.ReadBuildInfo) and standard environment
// variables (GIT_COMMIT, GIT_BRANCH).
func Git(configs ...GitConfig) monogo.ProcessorFunc {
	var cfg GitConfig
	if len(configs) > 0 {
		cfg = configs[0]
	} else {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					cfg.Commit = s.Value
				case "vcs.time":
					cfg.Time = s.Value
				case "vcs.modified":
					cfg.Modified = s.Value == "true"
				}
			}
		}
		if cfg.Commit == "" {
			cfg.Commit = os.Getenv("GIT_COMMIT")
		}
		if cfg.Branch == "" {
			cfg.Branch = os.Getenv("GIT_BRANCH")
		}
	}

	gitMap := make(map[string]interface{})
	if cfg.Commit != "" {
		gitMap["commit"] = cfg.Commit
	}
	if cfg.Branch != "" {
		gitMap["branch"] = cfg.Branch
	}
	if cfg.Time != "" {
		gitMap["time"] = cfg.Time
	}
	gitMap["modified"] = cfg.Modified

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["git"] = gitMap
		return r
	}
}

// Env adds specified environment variables to Extra["env"].
// Variables that are not set in the environment are omitted.
func Env(keys ...string) monogo.ProcessorFunc {
	return coreprocessor.Env(keys...)
}

// EnvMap maps environment variable names directly to top-level attributes in Extra.
// The map key is the environment variable name (e.g. "APP_ENV");
// the map value is the target key name in Extra (e.g. "environment").
func EnvMap(mapping map[string]string) monogo.ProcessorFunc {
	return coreprocessor.EnvMap(mapping)
}

