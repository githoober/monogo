package formatter

import (
	"bytes"
	"encoding/json"

	"github.com/githoober/monogo"
)

const (
	// BatchModeNewlines formats each record as an independent JSON line (NDJSON).
	BatchModeNewlines = 1
	// BatchModeJSON formats the batch as a single JSON array.
	BatchModeJSON = 2
)

// JSON formats log records into JSON.
type JSON struct {
	dateFormat string
	batchMode  int
}

// NewJSON creates a new JSON formatter defaulting to BatchModeNewlines.
func NewJSON(dateFormat string) *JSON {
	if dateFormat == "" {
		dateFormat = DefaultDateFormat
	}
	return &JSON{
		dateFormat: dateFormat,
		batchMode:  BatchModeNewlines,
	}
}

// WithBatchMode configures the batch formatting mode (BatchModeNewlines or BatchModeJSON).
func (j *JSON) WithBatchMode(mode int) *JSON {
	j.batchMode = mode
	return j
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

// FormatBatch formats a slice of Records according to the configured batch mode.
func (j *JSON) FormatBatch(records []monogo.Record) ([]byte, error) {
	if j.batchMode == BatchModeJSON {
		payloads := make([]jsonRecordPayload, len(records))
		for i, record := range records {
			payloads[i] = jsonRecordPayload{
				Message:   record.Message,
				Level:     record.Level.String(),
				LevelCode: int(record.Level),
				Channel:   record.Channel,
				Time:      record.Time.Format(j.dateFormat),
				Context:   record.Context,
				Extra:     record.Extra,
			}
		}
		data, err := json.Marshal(payloads)
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	}

	// Default: BatchModeNewlines (NDJSON)
	var buf bytes.Buffer
	for _, record := range records {
		b, err := j.Format(record)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}
