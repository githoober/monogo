package handler

import (
	"github.com/githoober/monogo"
)

// Filter wraps a handler and filters records based on level or predicate function.
type Filter struct {
	BaseHandler
	handler   monolog.Handler
	minLevel  monolog.Level
	maxLevel  monolog.Level
	predicate func(monolog.Record) bool
}

// NewFilter creates a Filter handler for level ranges [minLevel, maxLevel].
func NewFilter(handler monolog.Handler, minLevel, maxLevel monolog.Level) *Filter {
	return &Filter{
		BaseHandler: NewBaseHandler(minLevel, true),
		handler:     handler,
		minLevel:    minLevel,
		maxLevel:    maxLevel,
	}
}

// NewFilterFunc creates a Filter handler using custom predicate function.
func NewFilterFunc(handler monolog.Handler, predicate func(monolog.Record) bool) *Filter {
	return &Filter{
		BaseHandler: NewBaseHandler(monolog.DEBUG, true),
		handler:     handler,
		predicate:   predicate,
	}
}

// IsHandling checks if wrapped handler accepts record and record meets filter condition.
func (f *Filter) IsHandling(level monolog.Level) bool {
	if f.predicate == nil {
		if level < f.minLevel || level > f.maxLevel {
			return false
		}
	}
	return f.handler.IsHandling(level)
}

// Handle routes handling to inner handler if predicate/level check succeeds.
func (f *Filter) Handle(record monolog.Record) error {
	if f.predicate != nil {
		if !f.predicate(record) {
			return nil
		}
	} else {
		if record.Level < f.minLevel || record.Level > f.maxLevel {
			return nil
		}
	}

	return f.handler.Handle(record)
}

// Close closes wrapped handler.
func (f *Filter) Close() error {
	return f.handler.Close()
}
