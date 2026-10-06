package stdlogadapter

import (
	"bytes"
	"context"
	"io"
	"log"

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

// Writer implements io.Writer and forwards incoming log lines to a monogo.Logger at a designated level.
type Writer struct {
	ctx         context.Context
	logger      *monogo.Logger
	level       monogo.Level
	contextFunc func() context.Context
}

var _ io.Writer = (*Writer)(nil)

// NewWriter creates an io.Writer that forwards written lines to logger at the specified level.
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

// Write parses and emits the byte slice as a log record, trimming any trailing newlines.
func (w *Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	ctx := w.ctx
	if w.contextFunc != nil {
		if dynCtx := w.contextFunc(); dynCtx != nil {
			ctx = dynCtx
		}
	}

	line := string(bytes.TrimRight(p, "\r\n"))
	if err := w.logger.Log(ctx, w.level, line); err != nil {
		return 0, err
	}
	return len(p), nil
}

// NewStdLogger returns a standard library *log.Logger configured to write to logger at the specified level.
func NewStdLogger(ctx context.Context, logger *monogo.Logger, level monogo.Level, prefix string, flag int, opts ...Option) *log.Logger {
	w := NewWriter(ctx, logger, level, opts...)
	return log.New(w, prefix, flag)
}
