package handler

import (
	"io"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

// JSONStream writes log records formatted as JSON (NDJSON or JSON array) to an io.Writer.
type JSONStream struct {
	*Stream
}

// NewJSONStream creates a Stream handler pre-configured with formatter.NewJSON.
// It supports all standard Stream handler options (e.g. WithBubble, WithProcessor).
func NewJSONStream(w io.Writer, level monogo.Level, opts ...Option) *JSONStream {
	streamOpts := append([]Option{WithFormatter(formatter.NewJSON(""))}, opts...)
	return &JSONStream{
		Stream: NewStream(w, level, streamOpts...),
	}
}

// NewJSON is an alias for NewJSONStream.
func NewJSON(w io.Writer, level monogo.Level, opts ...Option) *JSONStream {
	return NewJSONStream(w, level, opts...)
}
