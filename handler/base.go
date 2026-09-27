package handler

import (
	"sync"

	"github.com/githoober/monogo"
)

// BaseHandler provides common functionality for handlers such as level handling and formatter management.
type BaseHandler struct {
	mu        sync.RWMutex
	level     monolog.Level
	formatter monolog.Formatter
	bubble    bool
}

// NewBaseHandler initializes a BaseHandler.
func NewBaseHandler(level monolog.Level, bubble bool) BaseHandler {
	return BaseHandler{
		level:  level,
		bubble: bubble,
	}
}

// IsHandling checks if record level meets minimum level threshold.
func (b *BaseHandler) IsHandling(level monolog.Level) bool {
	return level >= b.level
}

// SetLevel updates the minimum handling level.
func (b *BaseHandler) SetLevel(level monolog.Level) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.level = level
}

// SetFormatter sets the formatter.
func (b *BaseHandler) SetFormatter(formatter monolog.Formatter) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.formatter = formatter
}

// Formatter gets the current formatter.
func (b *BaseHandler) Formatter() monolog.Formatter {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.formatter
}

// Bubble returns whether handler allows bubbling.
func (b *BaseHandler) Bubble() bool {
	return b.bubble
}
