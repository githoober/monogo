package monogo

import "context"

// Handler handles a log record (e.g. writing to file, console, service, or forwarding to another handler).
type Handler interface {
	IsHandling(level Level) bool
	Handle(ctx context.Context, record Record) error
	Close() error
}

// BatchHandler is an optional interface for handlers capable of processing a batch of records
// in a single atomic or optimized operation.
type BatchHandler interface {
	Handler
	HandleBatch(ctx context.Context, records []Record) error
}

// Bubbler allows a handler to control whether a record bubbles down through the handler stack.
// When a handler processes a record and its Bubble() method returns false,
// the logger stops passing the record to subsequent handlers in the stack.
type Bubbler interface {
	Bubble() bool
}

// Processor enriches or modifies a log record before formatting and handling.
type Processor interface {
	Process(record Record) Record
}

// ProcessorFunc allows a function to be used as a Processor.
type ProcessorFunc func(record Record) Record

func (f ProcessorFunc) Process(record Record) Record {
	return f(record)
}

// Formatter formats a Record into bytes or string representation.
type Formatter interface {
	Format(record Record) ([]byte, error)
}

// BatchFormatter is an optional interface for formatters capable of formatting multiple
// records together (e.g. into a continuous buffer or a JSON array).
type BatchFormatter interface {
	Formatter
	FormatBatch(records []Record) ([]byte, error)
}

// ProcessableHandler represents a handler equipped with its own processor pipeline.
type ProcessableHandler interface {
	Handler
	Processors() []Processor
	ProcessRecord(record Record) Record
}

