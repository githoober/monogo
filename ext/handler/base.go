package handler

import (
	"context"
	"net"
	"time"

	"github.com/githoober/monogo"
	corehandler "github.com/githoober/monogo/handler"
)

// BaseHandler embeds corehandler.BaseHandler.
type BaseHandler = corehandler.BaseHandler

// Option configures handler options.
type Option func(*options)

// HandlerOption is an alias for Option.
type HandlerOption = Option

type options struct {
	bubble              bool
	formatter           monogo.Formatter
	processors          []monogo.Processor
	maxSizeMB           int
	maxBackups          int
	maxAgeDays          int
	compress            bool
	dedupKeyFunc        func(monogo.Record) string
	dedupStore          DeduplicationStore
	whatFailureCallback func(error, monogo.Handler)
	resetErrorCallback  func(error)
	sampler             func() bool
	samplingThreshold   *monogo.Level
	dialTimeout         time.Duration
	writeTimeout        time.Duration
	dialer              func(context.Context, string, string) (net.Conn, error)
}

func defaultOptions() options {
	return options{
		bubble:     true,
		maxSizeMB:  100,
		maxBackups: 3,
	}
}

func newBaseHandler(level monogo.Level, opts ...Option) (corehandler.BaseHandler, options) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	var coreOpts []corehandler.Option
	if !o.bubble {
		coreOpts = append(coreOpts, corehandler.WithBubble(false))
	}
	if o.formatter != nil {
		coreOpts = append(coreOpts, corehandler.WithFormatter(o.formatter))
	}
	if len(o.processors) > 0 {
		coreOpts = append(coreOpts, corehandler.WithProcessors(o.processors...))
	}
	return corehandler.NewBaseHandler(level, coreOpts...), o
}

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

// WithResetErrorCallback registers a callback invoked if an error occurs during Reset() (e.g. flushing a buffer).
func WithResetErrorCallback(fn func(error)) Option {
	return func(o *options) {
		o.resetErrorCallback = fn
	}
}

// WithSampler configures a custom sampling function (returns true if record should be emitted).
func WithSampler(sampler func() bool) Option {
	return func(o *options) {
		o.sampler = sampler
	}
}

// WithSamplingThreshold sets a level at or above which records bypass sampling and are always emitted.
func WithSamplingThreshold(level monogo.Level) Option {
	return func(o *options) {
		o.samplingThreshold = &level
	}
}

// WithDialTimeout configures connection dial timeout for network handlers.
func WithDialTimeout(d time.Duration) Option {
	return func(o *options) {
		o.dialTimeout = d
	}
}

// WithWriteTimeout configures network write timeout for network handlers.
func WithWriteTimeout(d time.Duration) Option {
	return func(o *options) {
		o.writeTimeout = d
	}
}

// WithDialer configures a custom connection dialer for network handlers.
func WithDialer(fn func(ctx context.Context, network, address string) (net.Conn, error)) Option {
	return func(o *options) {
		o.dialer = fn
	}
}

// NewTest creates a Test handler for assertions in tests.
func NewTest(level monogo.Level, opts ...Option) *corehandler.Test {
	base, _ := newBaseHandler(level, opts...)
	var coreOpts []corehandler.Option
	if !base.Bubble() {
		coreOpts = append(coreOpts, corehandler.WithBubble(false))
	}
	return corehandler.NewTest(level, coreOpts...)
}
