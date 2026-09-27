package formatter

import (
	"strings"

	"github.com/githoober/monogo"
)

// DefaultLineFormat is the standard line formatting template.
const DefaultLineFormat = "[%datetime%] %channel%.%level_name%: %message% %context% %extra%\n"
const DefaultDateFormat = "2006-01-02T15:04:05Z07:00"

// Line formats log records into a string template.
type Line struct {
	format     string
	dateFormat string
}

// NewLine creates a Line formatter with custom format and date format strings.
func NewLine(format, dateFormat string) *Line {
	if format == "" {
		format = DefaultLineFormat
	}
	if dateFormat == "" {
		dateFormat = DefaultDateFormat
	}
	return &Line{
		format:     format,
		dateFormat: dateFormat,
	}
}

// Format transforms a Record into formatted byte slice according to template.
func (f *Line) Format(record monogo.Record) ([]byte, error) {
	output := f.format

	output = strings.ReplaceAll(output, "%datetime%", record.Time.Format(f.dateFormat))
	output = strings.ReplaceAll(output, "%channel%", record.Channel)
	output = strings.ReplaceAll(output, "%level_name%", record.Level.String())
	output = strings.ReplaceAll(output, "%message%", record.Message)

	ctxStr := jsonifyMap(record.Context)
	output = strings.ReplaceAll(output, "%context%", ctxStr)

	extraStr := jsonifyMap(record.Extra)
	output = strings.ReplaceAll(output, "%extra%", extraStr)

	return []byte(output), nil
}

func jsonifyMap(m map[string]interface{}) string {
	if len(m) == 0 {
		return "[]"
	}
	data, err := monogo.MarshalJSONMap(m)
	if err != nil {
		return "[]"
	}
	return string(data)
}
