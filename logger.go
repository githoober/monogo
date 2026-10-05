package monogo

import (
	"context"
	"sync"
	"time"
)

type Logger struct {
	mu         sync.RWMutex
	cycleMu    *sync.RWMutex
	name       string
	handlers   []Handler
	processors []Processor
}

var _ Resettable = (*Logger)(nil)

func (l *Logger) getCycleMu() *sync.RWMutex {
	l.mu.RLock()
	m := l.cycleMu
	l.mu.RUnlock()
	if m != nil {
		return m
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cycleMu == nil {
		l.cycleMu = new(sync.RWMutex)
	}
	return l.cycleMu
}

func New(name string, handlers []Handler, processors []Processor) *Logger {
	if handlers == nil {
		handlers = make([]Handler, 0)
	}
	if processors == nil {
		processors = make([]Processor, 0)
	}
	return &Logger{
		cycleMu:    new(sync.RWMutex),
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
	cycleMu := l.cycleMu
	l.mu.RUnlock()

	if cycleMu == nil {
		cycleMu = l.getCycleMu()
	}

	return &Logger{
		cycleMu:    cycleMu,
		name:       name,
		handlers:   handlers,
		processors: processors,
	}
}

func (l *Logger) WithName(name string) *Logger {
	l.mu.RLock()
	handlers := make([]Handler, len(l.handlers))
	copy(handlers, l.handlers)

	processors := make([]Processor, len(l.processors))
	copy(processors, l.processors)

	cycleMu := l.cycleMu
	l.mu.RUnlock()

	if cycleMu == nil {
		cycleMu = l.getCycleMu()
	}

	return &Logger{
		cycleMu:    cycleMu,
		name:       name,
		handlers:   handlers,
		processors: processors,
	}
}

func (l *Logger) WithChannel(channel string) *Logger {
	return l.WithName(channel)
}

func (l *Logger) IsHandling(ctx context.Context, level Level) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, h := range l.handlers {
		if h.IsHandling(ctx, level) {
			return true
		}
	}
	return false
}

func (l *Logger) Log(ctx context.Context, level Level, msg string, ctxMap ...map[string]interface{}) error {
	if !l.IsHandling(ctx, level) {
		return nil
	}

	cycleMu := l.getCycleMu()
	cycleMu.RLock()
	defer cycleMu.RUnlock()

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
		if h.IsHandling(ctx, level) {
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
	cycleMu := l.getCycleMu()
	cycleMu.Lock()
	defer cycleMu.Unlock()

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

// Reset resets all handlers and processors that implement Resettable using the provided context.
// It acts as a strict lifecycle barrier: all in-flight Log calls complete before Reset begins,
// and incoming Log calls wait until Reset completes, preventing records from leaking across cycles.
// Returns the last error encountered during reset, if any.
func (l *Logger) Reset(ctx context.Context) error {
	cycleMu := l.getCycleMu()
	cycleMu.Lock()
	defer cycleMu.Unlock()

	l.mu.RLock()
	handlers := make([]Handler, len(l.handlers))
	copy(handlers, l.handlers)
	processors := make([]Processor, len(l.processors))
	copy(processors, l.processors)
	l.mu.RUnlock()

	var lastErr error
	for _, h := range handlers {
		if r, ok := h.(Resettable); ok {
			if err := r.Reset(ctx); err != nil {
				lastErr = err
			}
		}
	}
	for _, p := range processors {
		if r, ok := p.(Resettable); ok {
			if err := r.Reset(ctx); err != nil {
				lastErr = err
			}
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
