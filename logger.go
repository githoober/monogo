package monogo

import (
	"context"
	"sync"
	"time"
)

type Logger struct {
	mu         sync.RWMutex
	name       string
	handlers   []Handler
	processors []Processor
}

func New(name string, handlers []Handler, processors []Processor) *Logger {
	if handlers == nil {
		handlers = make([]Handler, 0)
	}
	if processors == nil {
		processors = make([]Processor, 0)
	}
	return &Logger{
		name:       name,
		handlers:   handlers,
		processors: processors,
	}
}

func (l *Logger) Name() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.name
}

func (l *Logger) PushHandler(handler Handler) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers = append([]Handler{handler}, l.handlers...)
	return l
}

func (l *Logger) PopHandler() (Handler, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.handlers) == 0 {
		return nil, false
	}
	h := l.handlers[0]
	l.handlers = l.handlers[1:]
	return h, true
}

func (l *Logger) PushProcessor(processor Processor) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.processors = append([]Processor{processor}, l.processors...)
	return l
}

func (l *Logger) With(ctxMap map[string]interface{}) *Logger {
	processor := ProcessorFunc(func(r Record) Record {
		if r.Context == nil {
			r.Context = make(map[string]interface{})
		}
		for k, v := range ctxMap {
			if _, exists := r.Context[k]; !exists {
				r.Context[k] = v
			}
		}
		return r
	})

	l.mu.RLock()
	handlers := make([]Handler, len(l.handlers))
	copy(handlers, l.handlers)
	processors := make([]Processor, len(l.processors)+1)
	processors[0] = processor
	copy(processors[1:], l.processors)
	name := l.name
	l.mu.RUnlock()

	return &Logger{
		name:       name,
		handlers:   handlers,
		processors: processors,
	}
}

func (l *Logger) WithName(name string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	handlers := make([]Handler, len(l.handlers))
	copy(handlers, l.handlers)

	processors := make([]Processor, len(l.processors))
	copy(processors, l.processors)

	return &Logger{
		name:       name,
		handlers:   handlers,
		processors: processors,
	}
}

func (l *Logger) WithChannel(channel string) *Logger {
	return l.WithName(channel)
}

func (l *Logger) IsHandling(level Level) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, h := range l.handlers {
		if h.IsHandling(level) {
			return true
		}
	}
	return false
}

func (l *Logger) Log(ctx context.Context, level Level, msg string, ctxMap ...map[string]interface{}) error {
	if !l.IsHandling(level) {
		return nil
	}

	mergedCtx := mergeContexts(FromContext(ctx), mergeContexts(ctxMap...))

	record := Record{
		Message: msg,
		Level:   level,
		Channel: l.name,
		Time:    time.Now(),
		Context: mergedCtx,
		Extra:   make(map[string]interface{}),
	}

	l.mu.RLock()
	processors := make([]Processor, len(l.processors))
	copy(processors, l.processors)
	handlers := make([]Handler, len(l.handlers))
	copy(handlers, l.handlers)
	l.mu.RUnlock()

	for _, p := range processors {
		record = p.Process(record)
	}

	for _, h := range handlers {
		if h.IsHandling(level) {
			if err := h.Handle(ctx, record); err != nil {
				return err
			}
			if bubbler, ok := h.(Bubbler); ok && !bubbler.Bubble() {
				break
			}
		}
	}

	return nil
}

func (l *Logger) Debug(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, DEBUG, msg, ctxMap...)
}

func (l *Logger) Info(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, INFO, msg, ctxMap...)
}

func (l *Logger) Notice(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, NOTICE, msg, ctxMap...)
}

func (l *Logger) Warning(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, WARNING, msg, ctxMap...)
}

func (l *Logger) Error(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, ERROR, msg, ctxMap...)
}

func (l *Logger) Critical(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, CRITICAL, msg, ctxMap...)
}

func (l *Logger) Alert(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, ALERT, msg, ctxMap...)
}

func (l *Logger) Emergency(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.Log(ctx, EMERGENCY, msg, ctxMap...)
}

func (l *Logger) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var lastErr error
	for _, h := range l.handlers {
		if err := h.Close(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func mergeContexts(ctxs ...map[string]interface{}) map[string]interface{} {
	if len(ctxs) == 0 {
		return nil
	}
	res := make(map[string]interface{})
	for _, c := range ctxs {
		for k, v := range c {
			res[k] = v
		}
	}
	return res
}
