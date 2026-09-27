package monolog

// Handler handles a log record (e.g. writing to file, console, service, or forwarding to another handler).
type Handler interface {
	IsHandling(level Level) bool
	Handle(record Record) error
	Close() error
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
