package handler

import (
	"github.com/githoober/monogo"
)

// Group forwards log records to a slice of handlers.
type Group struct {
	BaseHandler
	handlers []monolog.Handler
}

// NewGroup creates a Group handler with optional bubbling control (defaults to true).
func NewGroup(handlers []monolog.Handler, bubble ...bool) *Group {
	return &Group{
		BaseHandler: NewBaseHandler(monolog.DEBUG, bubble...),
		handlers:    handlers,
	}
}

// IsHandling returns true if any nested handler handles the log level.
func (g *Group) IsHandling(level monolog.Level) bool {
	for _, h := range g.handlers {
		if h.IsHandling(level) {
			return true
		}
	}
	return false
}

// Handle sends record to all sub-handlers that handle the record level.
func (g *Group) Handle(record monolog.Record) error {
	var lastErr error
	for _, h := range g.handlers {
		if h.IsHandling(record.Level) {
			if err := h.Handle(record); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// Close closes all nested handlers.
func (g *Group) Close() error {
	var lastErr error
	for _, h := range g.handlers {
		if err := h.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
