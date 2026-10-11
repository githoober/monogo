package formatter

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/githoober/monogo"
)

// DefaultLogstashDateFormat is the ISO 8601 format with millisecond precision expected by Logstash.
const DefaultLogstashDateFormat = "2006-01-02T15:04:05.000Z07:00"

// Logstash formats records into the Logstash Event V1 JSON format.
// Modeled after PHP Monolog's LogstashFormatter.
type Logstash struct {
	applicationName string
	systemName      string
	extraKey        string
	contextKey      string
	dateFormat      string
	batchMode       int
}

var _ monogo.Formatter = (*Logstash)(nil)
var _ monogo.BatchFormatter = (*Logstash)(nil)

// LogstashOption configures the Logstash formatter.
type LogstashOption func(*Logstash)

// WithLogstashSystemName sets the host/system name for Logstash event logs (defaults to os.Hostname()).
func WithLogstashSystemName(systemName string) LogstashOption {
	return func(l *Logstash) {
		l.systemName = systemName
	}
}

// WithLogstashExtraKey sets the key under which extra fields are stored (defaults to "extra").
func WithLogstashExtraKey(key string) LogstashOption {
	return func(l *Logstash) {
		if key != "" {
			l.extraKey = key
		}
	}
}

// WithLogstashContextKey sets the key under which context fields are stored (defaults to "context").
func WithLogstashContextKey(key string) LogstashOption {
	return func(l *Logstash) {
		if key != "" {
			l.contextKey = key
		}
	}
}

// WithLogstashDateFormat sets the timestamp format string (defaults to ISO 8601 millisecond format).
func WithLogstashDateFormat(format string) LogstashOption {
	return func(l *Logstash) {
		if format != "" {
			l.dateFormat = format
		}
	}
}

// WithLogstashBatchMode configures the batch formatting mode (BatchModeNewlines or BatchModeJSON).
func WithLogstashBatchMode(mode int) LogstashOption {
	return func(l *Logstash) {
		l.batchMode = mode
	}
}

// NewLogstash creates a new Logstash Event V1 formatter.
func NewLogstash(applicationName string, opts ...LogstashOption) *Logstash {
	host, _ := os.Hostname()
	l := &Logstash{
		applicationName: applicationName,
		systemName:      host,
		extraKey:        "extra",
		contextKey:      "context",
		dateFormat:      DefaultLogstashDateFormat,
		batchMode:       BatchModeNewlines,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	return l
}

// Format transforms a Record into a Logstash Event V1 JSON payload.
func (l *Logstash) Format(record monogo.Record) ([]byte, error) {
	payload := make(map[string]interface{})
	payload["@timestamp"] = record.Time.Format(l.dateFormat)
	payload["@version"] = 1

	if l.systemName != "" {
		payload["host"] = l.systemName
	}
	if record.Message != "" {
		payload["message"] = record.Message
	}
	if l.applicationName != "" {
		payload["type"] = l.applicationName
	} else if record.Channel != "" {
		payload["type"] = record.Channel
	}
	if record.Channel != "" {
		payload["channel"] = record.Channel
	}
	payload["level"] = record.Level.String()
	payload["monolog_level"] = int(record.Level)

	if len(record.Extra) > 0 {
		payload[l.extraKey] = record.Extra
	}
	if len(record.Context) > 0 {
		payload[l.contextKey] = record.Context
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// FormatBatch transforms a slice of Records into formatted bytes.
func (l *Logstash) FormatBatch(records []monogo.Record) ([]byte, error) {
	if len(records) == 0 {
		return []byte{}, nil
	}

	if l.batchMode == BatchModeJSON {
		var list []map[string]interface{}
		for _, r := range records {
			payload := make(map[string]interface{})
			payload["@timestamp"] = r.Time.Format(l.dateFormat)
			payload["@version"] = 1
			if l.systemName != "" {
				payload["host"] = l.systemName
			}
			if r.Message != "" {
				payload["message"] = r.Message
			}
			if l.applicationName != "" {
				payload["type"] = l.applicationName
			} else if r.Channel != "" {
				payload["type"] = r.Channel
			}
			if r.Channel != "" {
				payload["channel"] = r.Channel
			}
			payload["level"] = r.Level.String()
			payload["monolog_level"] = int(r.Level)
			if len(r.Extra) > 0 {
				payload[l.extraKey] = r.Extra
			}
			if len(r.Context) > 0 {
				payload[l.contextKey] = r.Context
			}
			list = append(list, payload)
		}
		data, err := json.Marshal(list)
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	}

	var buf bytes.Buffer
	for _, r := range records {
		line, err := l.Format(r)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
	}
	return buf.Bytes(), nil
}
