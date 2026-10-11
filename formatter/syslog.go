package formatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/githoober/monogo"
)

// Common Syslog facilities according to RFC 5424.
const (
	FacilityKern     = 0
	FacilityUser     = 1
	FacilityMail     = 2
	FacilityDaemon   = 3
	FacilityAuth     = 4
	FacilitySyslog   = 5
	FacilityLpr      = 6
	FacilityNews     = 7
	FacilityUucp     = 8
	FacilityCron     = 9
	FacilityAuthpriv = 10
	FacilityFtp      = 11
	FacilityLocal0   = 16
	FacilityLocal1   = 17
	FacilityLocal2   = 18
	FacilityLocal3   = 19
	FacilityLocal4   = 20
	FacilityLocal5   = 21
	FacilityLocal6   = 22
	FacilityLocal7   = 23
)

// Syslog formats log records into RFC 5424 syslog format.
// Modeled after PHP Monolog's SyslogFormatter.
type Syslog struct {
	appName    string
	hostname   string
	facility   int
	dateFormat string
	procID     int
}

var _ monogo.Formatter = (*Syslog)(nil)
var _ monogo.BatchFormatter = (*Syslog)(nil)

// SyslogOption configures the Syslog formatter.
type SyslogOption func(*Syslog)

// WithSyslogFacility configures the Syslog facility code (default: FacilityUser = 1).
func WithSyslogFacility(facility int) SyslogOption {
	return func(s *Syslog) {
		s.facility = facility
	}
}

// WithSyslogHostname configures the hostname (defaults to os.Hostname()).
func WithSyslogHostname(hostname string) SyslogOption {
	return func(s *Syslog) {
		if hostname != "" {
			s.hostname = hostname
		}
	}
}

// WithSyslogAppName configures the application name field in syslog header.
func WithSyslogAppName(appName string) SyslogOption {
	return func(s *Syslog) {
		if appName != "" {
			s.appName = appName
		}
	}
}

// WithSyslogDateFormat configures the timestamp layout (default: ISO 8601 millisecond format).
func WithSyslogDateFormat(format string) SyslogOption {
	return func(s *Syslog) {
		if format != "" {
			s.dateFormat = format
		}
	}
}

// NewSyslog creates a new Syslog formatter configured for RFC 5424 syslog output.
func NewSyslog(appName string, opts ...SyslogOption) *Syslog {
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	s := &Syslog{
		appName:    appName,
		hostname:   host,
		facility:   FacilityUser,
		dateFormat: DefaultLogstashDateFormat,
		procID:     os.Getpid(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func levelToSyslogSeverity(level monogo.Level) int {
	switch {
	case level >= monogo.EMERGENCY:
		return 0 // Emergency
	case level >= monogo.ALERT:
		return 1 // Alert
	case level >= monogo.CRITICAL:
		return 2 // Critical
	case level >= monogo.ERROR:
		return 3 // Error
	case level >= monogo.WARNING:
		return 4 // Warning
	case level >= monogo.NOTICE:
		return 5 // Notice
	case level >= monogo.INFO:
		return 6 // Informational
	default:
		return 7 // Debug
	}
}

// Format formats the record into an RFC 5424 syslog line.
func (s *Syslog) Format(record monogo.Record) ([]byte, error) {
	priority := (s.facility * 8) + levelToSyslogSeverity(record.Level)

	app := s.appName
	if app == "" {
		if record.Channel != "" {
			app = record.Channel
		} else {
			app = "-"
		}
	}

	msgID := "-"
	if record.Channel != "" {
		msgID = record.Channel
	}

	procIDStr := strconv.Itoa(s.procID)
	if s.procID <= 0 {
		procIDStr = "-"
	}

	msg := record.Message
	if len(record.Context) > 0 || len(record.Extra) > 0 {
		var metaParts []string
		if len(record.Context) > 0 {
			if ctxBytes, err := json.Marshal(record.Context); err == nil {
				metaParts = append(metaParts, string(ctxBytes))
			}
		}
		if len(record.Extra) > 0 {
			if extraBytes, err := json.Marshal(record.Extra); err == nil {
				metaParts = append(metaParts, string(extraBytes))
			}
		}
		if len(metaParts) > 0 {
			msg = fmt.Sprintf("%s %s", msg, metaParts)
		}
	}

	line := fmt.Sprintf("<%d>1 %s %s %s %s %s - %s\n",
		priority,
		record.Time.Format(s.dateFormat),
		s.hostname,
		app,
		procIDStr,
		msgID,
		msg,
	)

	return []byte(line), nil
}

// FormatBatch formats a slice of records into newline-delimited RFC 5424 syslog lines.
func (s *Syslog) FormatBatch(records []monogo.Record) ([]byte, error) {
	var buf bytes.Buffer
	for _, rec := range records {
		b, err := s.Format(rec)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}
