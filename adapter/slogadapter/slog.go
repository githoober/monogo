package slogadapter

import (
	"context"
	"log/slog"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

func ToSlogLevel(l monolog.Level) slog.Level {
	switch {
	case l < monolog.INFO:
		return slog.LevelDebug
	case l < monolog.WARNING:
		return slog.LevelInfo
	case l < monolog.ERROR:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

func FromSlogLevel(sl slog.Level) monolog.Level {
	switch {
	case sl < slog.LevelInfo:
		return monolog.DEBUG
	case sl < slog.LevelWarn:
		return monolog.INFO
	case sl < slog.LevelError:
		return monolog.WARNING
	default:
		return monolog.ERROR
	}
}

type SlogHandler struct {
	handler.BaseHandler
	slogHandler slog.Handler
}

// NewSlogHandler creates a SlogHandler with optional configuration options.
func NewSlogHandler(h slog.Handler, minLevel monolog.Level, opts ...handler.Option) *SlogHandler {
	return &SlogHandler{
		BaseHandler: handler.NewBaseHandler(minLevel, opts...),
		slogHandler: h,
	}
}

func (s *SlogHandler) Handle(record monolog.Record) error {
	slogLevel := ToSlogLevel(record.Level)
	if !s.slogHandler.Enabled(context.Background(), slogLevel) {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(record.Context)+len(record.Extra)+1)
	if record.Channel != "" {
		attrs = append(attrs, slog.String("channel", record.Channel))
	}

	for k, v := range record.Context {
		attrs = append(attrs, slog.Any(k, v))
	}

	for k, v := range record.Extra {
		attrs = append(attrs, slog.Any("extra."+k, v))
	}

	r := slog.NewRecord(record.Time, slogLevel, record.Message, 0)
	r.AddAttrs(attrs...)

	return s.slogHandler.Handle(context.Background(), r)
}

func (s *SlogHandler) Close() error {
	return nil
}

type MonologSlogBridge struct {
	logger *monolog.Logger
	attrs  []slog.Attr
	group  string
}

func NewMonologSlogBridge(logger *monolog.Logger) *MonologSlogBridge {
	return &MonologSlogBridge{
		logger: logger,
	}
}

func (m *MonologSlogBridge) Enabled(ctx context.Context, level slog.Level) bool {
	return m.logger.IsHandling(FromSlogLevel(level))
}

func (m *MonologSlogBridge) Handle(ctx context.Context, r slog.Record) error {
	ctxMap := make(map[string]interface{})

	for _, attr := range m.attrs {
		m.addAttrToMap(ctxMap, attr)
	}

	r.Attrs(func(attr slog.Attr) bool {
		m.addAttrToMap(ctxMap, attr)
		return true
	})

	lvl := FromSlogLevel(r.Level)
	return m.logger.LogContext(ctx, lvl, r.Message, ctxMap)
}

func (m *MonologSlogBridge) addAttrToMap(target map[string]interface{}, attr slog.Attr) {
	key := attr.Key
	if m.group != "" {
		key = m.group + "." + key
	}
	target[key] = attr.Value.Any()
}

func (m *MonologSlogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(m.attrs)+len(attrs))
	copy(newAttrs, m.attrs)
	copy(newAttrs[len(m.attrs):], attrs)

	return &MonologSlogBridge{
		logger: m.logger,
		attrs:  newAttrs,
		group:  m.group,
	}
}

func (m *MonologSlogBridge) WithGroup(name string) slog.Handler {
	if name == "" {
		return m
	}
	newGroup := name
	if m.group != "" {
		newGroup = m.group + "." + name
	}

	return &MonologSlogBridge{
		logger: m.logger,
		attrs:  m.attrs,
		group:  newGroup,
	}
}
