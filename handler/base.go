package handler

import (
	"github.com/githoober/monogo"
)

// BaseHandler provides common functionality for handlers such as level handling and formatter management.
type BaseHandler struct {
	level     monogo.Level
	formatter monogo.Formatter
	bubble    bool
}

type options struct {
	bubble     bool
	formatter  monogo.Formatter
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

// NewBaseHandler initializes a BaseHandler with optional configuration options.
func NewBaseHandler(level monogo.Level, opts ...Option) BaseHandler {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return BaseHandler{
		level:     level,
		formatter: o.formatter,
		bubble:    o.bubble,
	}
}

// IsHandling checks if record level meets minimum level threshold.
func (b *BaseHandler) IsHandling(level monogo.Level) bool {
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
