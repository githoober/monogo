package handler

import (
	"context"

	"github.com/githoober/monogo"
)

// BaseHandler provides common functionality for handlers such as level handling,
// formatter management, and per-handler processor execution.
type BaseHandler struct {
	level      monogo.Level
	formatter  monogo.Formatter
	bubble     bool
	processors []monogo.Processor
}

type options struct {
	bubble     bool
	formatter  monogo.Formatter
	processors []monogo.Processor
	maxSizeMB  int
	maxBackups int
	maxAgeDays int
	compress   bool
}

func defaultOptions() options {
	return options{
		bubble:     true,
		maxSizeMB:  100,
		maxBackups: 3,
	}
}

// Option configures handler behavior.
type Option func(*options)

// HandlerOption is an alias for Option.
type HandlerOption = Option

// WithBubble configures whether the handler allows record bubbling down the stack.
// Defaults to true when omitted.
func WithBubble(bubble bool) Option {
	return func(o *options) {
		o.bubble = bubble
	}
}

// WithFormatter configures the handler's formatter at construction time.
func WithFormatter(formatter monogo.Formatter) Option {
	return func(o *options) {
		o.formatter = formatter
	}
}

// WithProcessor configures one or more processors to execute on this handler before formatting/dispatching.
func WithProcessor(processors ...monogo.Processor) Option {
	return func(o *options) {
		o.processors = append(o.processors, processors...)
	}
}

// WithProcessors is an alias for WithProcessor to configure multiple processors at construction time.
func WithProcessors(processors ...monogo.Processor) Option {
	return WithProcessor(processors...)
}

// NewBaseHandler initializes a BaseHandler with optional configuration options.
func NewBaseHandler(level monogo.Level, opts ...Option) BaseHandler {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	var procs []monogo.Processor
	if len(o.processors) > 0 {
		procs = make([]monogo.Processor, len(o.processors))
		copy(procs, o.processors)
	}
	return BaseHandler{
		level:      level,
		formatter:  o.formatter,
		bubble:     o.bubble,
		processors: procs,
	}
}

// IsHandling checks if record level meets minimum level threshold.
func (b *BaseHandler) IsHandling(_ context.Context, level monogo.Level) bool {
	return level >= b.level
}

// Formatter gets the current formatter.
func (b *BaseHandler) Formatter() monogo.Formatter {
	return b.formatter
}

// Bubble returns whether handler allows bubbling.
func (b *BaseHandler) Bubble() bool {
	return b.bubble
}

// Processors returns a copy of the handler's processor pipeline.
func (b *BaseHandler) Processors() []monogo.Processor {
	if b == nil || len(b.processors) == 0 {
		return nil
	}
	procs := make([]monogo.Processor, len(b.processors))
	copy(procs, b.processors)
	return procs
}

// ProcessRecord applies the handler's processor pipeline to the record.
// If processors are present, the record is cloned first to prevent mutations from
// leaking to subsequent handlers down the logger stack.
func (b *BaseHandler) ProcessRecord(record monogo.Record) monogo.Record {
	if b == nil || len(b.processors) == 0 {
		return record
	}
	record = record.Clone()
	for _, p := range b.processors {
		record = p.Process(record)
	}
	return record
}
