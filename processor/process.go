package processor

import (
	"os"

	"github.com/githoober/monogo"
)

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

// Process is an alias for ProcessId.
func Process() monogo.ProcessorFunc {
	return ProcessId()
}
