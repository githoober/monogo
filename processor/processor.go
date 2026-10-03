package processor

import (
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/githoober/monogo"
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

// UID generates a random 16-character hex unique identifier string and adds it to Extra["uid"].
func UID() monogo.ProcessorFunc {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	uid := fmt.Sprintf("%x", b)

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["uid"] = uid
		return r
	}
}

// ProcessId adds the operating system process ID (os.Getpid()) to Extra["pid"].
func ProcessId() monogo.ProcessorFunc {
	pid := os.Getpid()
	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		r.Extra["pid"] = pid
		return r
	}
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
	envMap := make(map[string]interface{}, len(keys))
	for _, k := range keys {
		if val, exists := os.LookupEnv(k); exists {
			envMap[k] = val
		}
	}

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		if existing, ok := r.Extra["env"].(map[string]interface{}); ok {
			for k, v := range envMap {
				existing[k] = v
			}
		} else {
			cp := make(map[string]interface{}, len(envMap))
			for k, v := range envMap {
				cp[k] = v
			}
			r.Extra["env"] = cp
		}
		return r
	}
}

// EnvMap maps environment variable names directly to top-level attributes in Extra.
// The map key is the environment variable name (e.g. "APP_ENV");
// the map value is the target key name in Extra (e.g. "environment").
func EnvMap(mapping map[string]string) monogo.ProcessorFunc {
	mapped := make(map[string]interface{}, len(mapping))
	for envVar, extraKey := range mapping {
		if val, exists := os.LookupEnv(envVar); exists {
			mapped[extraKey] = val
		}
	}

	return func(r monogo.Record) monogo.Record {
		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}
		for k, v := range mapped {
			r.Extra[k] = v
		}
		return r
	}
}

