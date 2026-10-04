package formatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/githoober/monogo"
)

// Logfmt formats log records into canonical key=value logfmt pairs.
//
// Example output:
//
//	ts=2026-10-04T12:00:00Z lvl=INFO channel=app msg="User logged in" user_id=42
type Logfmt struct {
	timeKey        string
	levelKey       string
	channelKey     string
	messageKey     string
	dateFormat     string
	contextPrefix  string
	extraPrefix    string
	lowercaseLevel bool
}

// LogfmtFormatter is an alias for Logfmt.
type LogfmtFormatter = Logfmt

// LogfmtOption configures the Logfmt formatter.
type LogfmtOption func(*Logfmt)

// WithTimeKey configures the key name used for the record timestamp.
// If set to empty string, the timestamp field is omitted. Defaults to "ts".
func WithTimeKey(key string) LogfmtOption {
	return func(f *Logfmt) {
		f.timeKey = key
	}
}

// WithLevelKey configures the key name used for the record severity level.
// If set to empty string, the level field is omitted. Defaults to "lvl".
func WithLevelKey(key string) LogfmtOption {
	return func(f *Logfmt) {
		f.levelKey = key
	}
}

// WithChannelKey configures the key name used for the logger channel name.
// If set to empty string, the channel field is omitted. Defaults to "channel".
func WithChannelKey(key string) LogfmtOption {
	return func(f *Logfmt) {
		f.channelKey = key
	}
}

// WithMessageKey configures the key name used for the log message.
// If set to empty string, the message field is omitted. Defaults to "msg".
func WithMessageKey(key string) LogfmtOption {
	return func(f *Logfmt) {
		f.messageKey = key
	}
}

// WithDateFormat configures the timestamp layout. Defaults to DefaultDateFormat (RFC3339).
func WithDateFormat(dateFormat string) LogfmtOption {
	return func(f *Logfmt) {
		if dateFormat != "" {
			f.dateFormat = dateFormat
		}
	}
}

// WithContextPrefix configures a prefix prepended to all context keys (e.g. "ctx.").
func WithContextPrefix(prefix string) LogfmtOption {
	return func(f *Logfmt) {
		f.contextPrefix = prefix
	}
}

// WithExtraPrefix configures a prefix prepended to all processor extra keys (e.g. "extra.").
func WithExtraPrefix(prefix string) LogfmtOption {
	return func(f *Logfmt) {
		f.extraPrefix = prefix
	}
}

// WithLowercaseLevel configures whether the severity level name is formatted in lowercase (e.g. "info" vs "INFO").
func WithLowercaseLevel(lowercase bool) LogfmtOption {
	return func(f *Logfmt) {
		f.lowercaseLevel = lowercase
	}
}

// Compile-time interface checks.
var (
	_ monogo.Formatter      = (*Logfmt)(nil)
	_ monogo.BatchFormatter = (*Logfmt)(nil)
)

// NewLogfmt creates a new Logfmt formatter with optional configuration options.
func NewLogfmt(opts ...LogfmtOption) *Logfmt {
	f := &Logfmt{
		timeKey:    "ts",
		levelKey:   "lvl",
		channelKey: "channel",
		messageKey: "msg",
		dateFormat: DefaultDateFormat,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(f)
		}
	}
	return f
}

// Format formats a single Record into a logfmt byte slice ending with a newline.
func (f *Logfmt) Format(record monogo.Record) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Timestamp
	if f.timeKey != "" {
		buf.WriteString(formatKey(f.timeKey))
		buf.WriteByte('=')
		buf.WriteString(formatString(record.Time.Format(f.dateFormat)))
	}

	// 2. Level
	if f.levelKey != "" {
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(formatKey(f.levelKey))
		buf.WriteByte('=')
		lvlStr := record.Level.String()
		if f.lowercaseLevel {
			lvlStr = strings.ToLower(lvlStr)
		}
		buf.WriteString(formatString(lvlStr))
	}

	// 3. Channel
	if f.channelKey != "" && record.Channel != "" {
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(formatKey(f.channelKey))
		buf.WriteByte('=')
		buf.WriteString(formatString(record.Channel))
	}

	// 4. Message
	if f.messageKey != "" {
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(formatKey(f.messageKey))
		buf.WriteByte('=')
		buf.WriteString(formatString(record.Message))
	}

	// 5. Context
	if len(record.Context) > 0 {
		keys := make([]string, 0, len(record.Context))
		for k := range record.Context {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(formatKey(f.contextPrefix + k))
			buf.WriteByte('=')
			buf.WriteString(formatValue(record.Context[k]))
		}
	}

	// 6. Extra
	if len(record.Extra) > 0 {
		keys := make([]string, 0, len(record.Extra))
		for k := range record.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(formatKey(f.extraPrefix + k))
			buf.WriteByte('=')
			buf.WriteString(formatValue(record.Extra[k]))
		}
	}

	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// FormatBatch formats a slice of Records into a continuous byte slice of newline-delimited logfmt lines.
func (f *Logfmt) FormatBatch(records []monogo.Record) ([]byte, error) {
	var buf bytes.Buffer
	for _, record := range records {
		b, err := f.Format(record)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

func formatKey(key string) string {
	if key == "" {
		return "_"
	}
	var buf strings.Builder
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c <= ' ' || c == '=' || c == '"' || c == '\\' || c == 0x7f {
			buf.WriteByte('_')
		} else {
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

func formatString(s string) string {
	if s == "" {
		return `""`
	}
	if needsQuoting(s) {
		return strconv.Quote(s)
	}
	return s
}

func needsQuoting(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c == '=' || c == '"' || c == '\\' || c == 0x7f {
			return true
		}
	}
	return false
}

func formatValue(val interface{}) string {
	if val == nil {
		return "null"
	}
	switch v := val.(type) {
	case string:
		return formatString(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case time.Time:
		return formatString(v.Format(time.RFC3339Nano))
	case time.Duration:
		return formatString(v.String())
	case []byte:
		return formatString(string(v))
	case error:
		return formatString(v.Error())
	case fmt.Stringer:
		return formatString(v.String())
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return formatString(fmt.Sprint(v))
		}
		return formatString(string(b))
	}
}
