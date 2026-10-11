package handler

import (
	"fmt"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

// SyslogUdp streams formatted log records to a remote syslog daemon over UDP.
// Modeled after PHP Monolog's SyslogUdpHandler.
type SyslogUdp struct {
	*Socket
}

// SyslogUdpHandler is an alias for SyslogUdp.
type SyslogUdpHandler = SyslogUdp

// Compile-time interface assertions.
var (
	_ monogo.Handler            = (*SyslogUdp)(nil)
	_ monogo.BatchHandler       = (*SyslogUdp)(nil)
	_ monogo.Bubbler            = (*SyslogUdp)(nil)
	_ monogo.ProcessableHandler = (*SyslogUdp)(nil)
	_ monogo.Resettable         = (*SyslogUdp)(nil)
)

// NewSyslogUdp creates a SyslogUdp handler streaming formatted syslog records to host:port over UDP.
// If port is 0, it defaults to the standard syslog port 514.
func NewSyslogUdp(host string, port int, minLevel monogo.Level, opts ...Option) *SyslogUdp {
	if port <= 0 {
		port = 514
	}
	address := fmt.Sprintf("%s:%d", host, port)

	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	if o.formatter == nil {
		opts = append([]Option{WithFormatter(formatter.NewSyslog("monogo"))}, opts...)
	}

	sock := NewSocket("udp", address, minLevel, opts...)
	return &SyslogUdp{
		Socket: sock,
	}
}
