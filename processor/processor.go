package processor

import (
	"crypto/rand"
	"fmt"
	"os"
	"runtime"

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
