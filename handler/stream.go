package handler

import (
	"io"
	"sync"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

// Stream writes formatted log records to an io.Writer.
type Stream struct {
	BaseHandler
	writer io.Writer
	mu     sync.Mutex
}

// NewStream creates a Stream handler with optional configuration options (defaults: bubble=true).
func NewStream(w io.Writer, level monolog.Level, opts ...Option) *Stream {
	h := &Stream{
		BaseHandler: NewBaseHandler(level, opts...),
		writer:      w,
	}
	h.SetFormatter(formatter.NewLine("", ""))
	return h
}

// Handle formats and writes the record to stream writer.
func (s *Stream) Handle(record monolog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f := s.Formatter()
	if f == nil {
		f = formatter.NewLine("", "")
	}

	bytes, err := f.Format(record)
	if err != nil {
		return err
	}

	_, err = s.writer.Write(bytes)
	return err
}

// Close closes the stream writer if it implements io.Closer.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if closer, ok := s.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
