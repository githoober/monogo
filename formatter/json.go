package formatter

import (
	"encoding/json"

	"github.com/githoober/monogo"
)

// JSON formats log records into JSON.
type JSON struct {
	dateFormat string
}

// NewJSON creates a new JSON formatter.
func NewJSON(dateFormat string) *JSON {
	if dateFormat == "" {
		dateFormat = DefaultDateFormat
	}
	return &JSON{
		dateFormat: dateFormat,
	}
}

type jsonRecordPayload struct {
	Message   string                 `json:"message"`
	Level     string                 `json:"level_name"`
	LevelCode int                    `json:"level"`
	Channel   string                 `json:"channel"`
	Time      string                 `json:"datetime"`
	Context   map[string]interface{} `json:"context,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// Format formats the record as JSON bytes.
func (j *JSON) Format(record monogo.Record) ([]byte, error) {
	payload := jsonRecordPayload{
		Message:   record.Message,
		Level:     record.Level.String(),
		LevelCode: int(record.Level),
		Channel:   record.Channel,
		Time:      record.Time.Format(j.dateFormat),
		Context:   record.Context,
		Extra:     record.Extra,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}
