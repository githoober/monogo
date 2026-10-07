package stdlogadapter

import (
	"bytes"
	"context"
	"io"
	"log"
	"sync"

	"github.com/githoober/monogo"
)

// Option configures Writer behavior.
type Option func(*options)

type options struct {
	contextFunc func() context.Context
}

// WithContextFunc configures a dynamic context retrieval function for the Writer.
// If provided, the Writer invokes this function for each Write call. If it returns
// a non-nil context, that context is used instead of the base context.
func WithContextFunc(fn func() context.Context) Option {
	return func(o *options) {
		o.contextFunc = fn
	}
}

// Writer implements io.Writer and io.Closer, framing incoming byte streams into individual
// log lines and forwarding them to a monogo.Logger at a designated level.
type Writer struct {
	ctx         context.Context
	logger      *monogo.Logger
	level       monogo.Level
	contextFunc func() context.Context

	mu  sync.Mutex
	buf []byte
}

var (
	_ io.Writer = (*Writer)(nil)
	_ io.Closer = (*Writer)(nil)
)

// NewWriter creates an io.Writer that frames written byte streams into individual log lines
// and forwards them to logger at the specified level.
// ctx must be a non-nil context provided by the caller (application lifecycle, server startup, or request context).
func NewWriter(ctx context.Context, logger *monogo.Logger, level monogo.Level, opts ...Option) *Writer {
	o := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &Writer{
		ctx:         ctx,
		logger:      logger,
		level:       level,
		contextFunc: o.contextFunc,
	}
}

// Write buffers incoming bytes, extracts newline-terminated log lines, and forwards each
// line as a log record. Partial lines without a terminating newline remain buffered until
// subsequent writes provide a newline or Flush/Close is invoked.
func (w *Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		lineBytes := w.buf[:idx]
		line := string(bytes.TrimRight(lineBytes, "\r"))
		w.buf = w.buf[idx+1:]

		ctx := w.ctx
		if w.contextFunc != nil {
			if dynCtx := w.contextFunc(); dynCtx != nil {
				ctx = dynCtx
			}
		}

		if err := w.logger.Log(ctx, w.level, line); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

// Flush flushes any remaining buffered partial line to the logger.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buf) == 0 {
		return nil
	}
	line := string(bytes.TrimRight(w.buf, "\r\n"))
	w.buf = nil

	ctx := w.ctx
	if w.contextFunc != nil {
		if dynCtx := w.contextFunc(); dynCtx != nil {
			ctx = dynCtx
		}
	}
	return w.logger.Log(ctx, w.level, line)
}

// Close flushes any pending buffered line.
func (w *Writer) Close() error {
	return w.Flush()
}

// NewStdLogger returns a standard library *log.Logger configured to write to logger at the specified level.
func NewStdLogger(ctx context.Context, logger *monogo.Logger, level monogo.Level, prefix string, flag int, opts ...Option) *log.Logger {
	w := NewWriter(ctx, logger, level, opts...)
	return log.New(w, prefix, flag)
}
