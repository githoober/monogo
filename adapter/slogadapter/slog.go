package slogadapter

import (
	"context"
	"log/slog"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

// Custom slog levels corresponding to RFC 5424 / monogo levels not natively defined in log/slog.
// Slog uses 4-step spacing between standard levels (Debug=-4, Info=0, Warn=4, Error=8),
// which explicitly supports defining custom intermediate and extended levels.
const (
	LevelNotice    slog.Level = slog.LevelInfo + 2  // 2 (between Info and Warn)
	LevelCritical  slog.Level = slog.LevelError + 4 // 12 (above Error)
	LevelAlert     slog.Level = slog.LevelError + 8 // 16 (above Critical)
	LevelEmergency slog.Level = slog.LevelError + 12 // 20 (above Alert)
)

// ReplaceLevelAttr is a slog ReplaceAttr helper that formats custom RFC 5424 levels
// (Notice, Critical, Alert, Emergency) as human-readable string values instead of "INFO+2", etc.
func ReplaceLevelAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		if lvl, ok := a.Value.Any().(slog.Level); ok {
			switch lvl {
			case LevelNotice:
				a.Value = slog.StringValue("NOTICE")
			case LevelCritical:
				a.Value = slog.StringValue("CRITICAL")
			case LevelAlert:
				a.Value = slog.StringValue("ALERT")
			case LevelEmergency:
				a.Value = slog.StringValue("EMERGENCY")
			}
		}
	}
	return a
}

func ToSlogLevel(l monogo.Level) slog.Level {
	switch {
	case l < monogo.INFO:
		return slog.LevelDebug
	case l < monogo.NOTICE:
		return slog.LevelInfo
	case l < monogo.WARNING:
		return LevelNotice
	case l < monogo.ERROR:
		return slog.LevelWarn
	case l < monogo.CRITICAL:
		return slog.LevelError
	case l < monogo.ALERT:
		return LevelCritical
	case l < monogo.EMERGENCY:
		return LevelAlert
	default:
		return LevelEmergency
	}
}

func FromSlogLevel(sl slog.Level) monogo.Level {
	switch {
	case sl < slog.LevelInfo:
		return monogo.DEBUG
	case sl < LevelNotice:
		return monogo.INFO
	case sl < slog.LevelWarn:
		return monogo.NOTICE
	case sl < slog.LevelError:
		return monogo.WARNING
	case sl < LevelCritical:
		return monogo.ERROR
	case sl < LevelAlert:
		return monogo.CRITICAL
	case sl < LevelEmergency:
		return monogo.ALERT
	default:
		return monogo.EMERGENCY
	}
}

type SlogHandler struct {
	handler.BaseHandler
	slogHandler slog.Handler
}

// NewSlogHandler creates a SlogHandler with optional configuration options.
func NewSlogHandler(h slog.Handler, minLevel monogo.Level, opts ...handler.Option) *SlogHandler {
	return &SlogHandler{
		BaseHandler: handler.NewBaseHandler(minLevel, opts...),
		slogHandler: h,
	}
}

func (s *SlogHandler) Handle(ctx context.Context, record monogo.Record) error {
	record = s.ProcessRecord(record)
	slogLevel := ToSlogLevel(record.Level)
	if !s.slogHandler.Enabled(ctx, slogLevel) {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(record.Context)+len(record.Extra)+1)
	if record.Channel != "" {
		attrs = append(attrs, slog.String("channel", record.Channel))
	}

	if record.Level == monogo.NOTICE || record.Level >= monogo.CRITICAL {
		attrs = append(attrs, slog.String("severity", record.Level.String()))
	}

	for k, v := range record.Context {
		attrs = append(attrs, slog.Any(k, v))
	}

	for k, v := range record.Extra {
		attrs = append(attrs, slog.Any("extra."+k, v))
	}

	r := slog.NewRecord(record.Time, slogLevel, record.Message, 0)
	r.AddAttrs(attrs...)

	return s.slogHandler.Handle(ctx, r)
}

func (s *SlogHandler) Close(ctx context.Context) error {
	return nil
}

type MonogoSlogBridge struct {
	logger *monogo.Logger
	attrs  []slog.Attr
	group  string
}

func NewMonogoSlogBridge(logger *monogo.Logger) *MonogoSlogBridge {
	return &MonogoSlogBridge{
		logger: logger,
	}
}

// MonologSlogBridge is an alias for MonogoSlogBridge for backwards compatibility.
type MonologSlogBridge = MonogoSlogBridge

// NewMonologSlogBridge creates a MonogoSlogBridge (alias for backwards compatibility).
func NewMonologSlogBridge(logger *monogo.Logger) *MonogoSlogBridge {
	return NewMonogoSlogBridge(logger)
}

func (m *MonogoSlogBridge) Enabled(ctx context.Context, level slog.Level) bool {
	return m.logger.IsHandling(FromSlogLevel(level))
}

func (m *MonogoSlogBridge) Handle(ctx context.Context, r slog.Record) error {
	ctxMap := make(map[string]interface{})

	for _, attr := range m.attrs {
		m.addAttrToMap(ctxMap, attr)
	}

	r.Attrs(func(attr slog.Attr) bool {
		m.addAttrToMap(ctxMap, attr)
		return true
	})

	lvl := FromSlogLevel(r.Level)
	return m.logger.Log(ctx, lvl, r.Message, ctxMap)
}

func (m *MonologSlogBridge) addAttrToMap(target map[string]interface{}, attr slog.Attr) {
	key := attr.Key
	if m.group != "" {
		key = m.group + "." + key
	}
	target[key] = attr.Value.Any()
}

func (m *MonogoSlogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(m.attrs)+len(attrs))
	copy(newAttrs, m.attrs)
	copy(newAttrs[len(m.attrs):], attrs)

	return &MonogoSlogBridge{
		logger: m.logger,
		attrs:  newAttrs,
		group:  m.group,
	}
}

func (m *MonogoSlogBridge) WithGroup(name string) slog.Handler {
	if name == "" {
		return m
	}
	newGroup := name
	if m.group != "" {
		newGroup = m.group + "." + name
	}

	return &MonogoSlogBridge{
		logger: m.logger,
		attrs:  m.attrs,
		group:  newGroup,
	}
}
