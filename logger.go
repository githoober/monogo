package monogo

import (
	"context"
	"fmt"
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

func (l *Logger) Log(level Level, msg string, ctx map[string]interface{}) error {
	return l.LogContext(context.Background(), level, msg, ctx)
}

func (l *Logger) Logf(level Level, format string, args ...interface{}) error {
	return l.Log(level, fmt.Sprintf(format, args...), nil)
}

func (l *Logger) LogContext(ctx context.Context, level Level, msg string, ctxMap map[string]interface{}) error {
	if !l.IsHandling(level) {
		return nil
	}

	mergedCtx := mergeContexts(FromContext(ctx), ctxMap)

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
			if err := h.Handle(record); err != nil {
				return err
			}
			if bubbler, ok := h.(Bubbler); ok && !bubbler.Bubble() {
				break
			}
		}
	}

	return nil
}

func (l *Logger) Debug(msg string, ctx ...map[string]interface{}) error {
	return l.Log(DEBUG, msg, mergeContexts(ctx...))
}

func (l *Logger) Info(msg string, ctx ...map[string]interface{}) error {
	return l.Log(INFO, msg, mergeContexts(ctx...))
}

func (l *Logger) Notice(msg string, ctx ...map[string]interface{}) error {
	return l.Log(NOTICE, msg, mergeContexts(ctx...))
}

func (l *Logger) Warning(msg string, ctx ...map[string]interface{}) error {
	return l.Log(WARNING, msg, mergeContexts(ctx...))
}

func (l *Logger) Error(msg string, ctx ...map[string]interface{}) error {
	return l.Log(ERROR, msg, mergeContexts(ctx...))
}

func (l *Logger) Critical(msg string, ctx ...map[string]interface{}) error {
	return l.Log(CRITICAL, msg, mergeContexts(ctx...))
}

func (l *Logger) Alert(msg string, ctx ...map[string]interface{}) error {
	return l.Log(ALERT, msg, mergeContexts(ctx...))
}

func (l *Logger) Emergency(msg string, ctx ...map[string]interface{}) error {
	return l.Log(EMERGENCY, msg, mergeContexts(ctx...))
}

func (l *Logger) DebugContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, DEBUG, msg, mergeContexts(ctxMap...))
}

func (l *Logger) InfoContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, INFO, msg, mergeContexts(ctxMap...))
}

func (l *Logger) NoticeContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, NOTICE, msg, mergeContexts(ctxMap...))
}

func (l *Logger) WarningContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, WARNING, msg, mergeContexts(ctxMap...))
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, ERROR, msg, mergeContexts(ctxMap...))
}

func (l *Logger) CriticalContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, CRITICAL, msg, mergeContexts(ctxMap...))
}

func (l *Logger) AlertContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, ALERT, msg, mergeContexts(ctxMap...))
}

func (l *Logger) EmergencyContext(ctx context.Context, msg string, ctxMap ...map[string]interface{}) error {
	return l.LogContext(ctx, EMERGENCY, msg, mergeContexts(ctxMap...))
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var lastErr error
	for _, h := range l.handlers {
		if err := h.Close(); err != nil {
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
