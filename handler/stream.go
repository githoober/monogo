package handler

import (
	"bytes"
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
func NewStream(w io.Writer, level monogo.Level, opts ...Option) *Stream {
	h := &Stream{
		BaseHandler: NewBaseHandler(level, opts...),
		writer:      w,
	}
	if h.formatter == nil {
		h.formatter = formatter.NewLine("", "")
	}
	return h
}

// Handle formats and writes the record to stream writer.
func (s *Stream) Handle(record monogo.Record) error {
	record = s.ProcessRecord(record)

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

// HandleBatch formats and writes a batch of records to the stream writer in a single operation.
func (s *Stream) HandleBatch(records []monogo.Record) error {
	handled := make([]monogo.Record, 0, len(records))
	for _, rec := range records {
		if s.IsHandling(rec.Level) {
			handled = append(handled, s.ProcessRecord(rec))
		}
	}
	if len(handled) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f := s.Formatter()
	if f == nil {
		f = formatter.NewLine("", "")
	}

	var payload []byte
	if bf, ok := f.(monogo.BatchFormatter); ok {
		var err error
		payload, err = bf.FormatBatch(handled)
		if err != nil {
			return err
		}
	} else {
		var buf bytes.Buffer
		for _, rec := range handled {
			b, err := f.Format(rec)
			if err != nil {
				return err
			}
			buf.Write(b)
		}
		payload = buf.Bytes()
	}

	_, err := s.writer.Write(payload)
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
