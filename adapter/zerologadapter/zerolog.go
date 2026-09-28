package zerologadapter

import (
	"context"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/rs/zerolog"
)

type ZerologHandler struct {
	handler.BaseHandler
	logger zerolog.Logger
}

// NewZerologHandler creates a ZerologHandler with optional configuration options.
func NewZerologHandler(l zerolog.Logger, minLevel monogo.Level, opts ...handler.Option) *ZerologHandler {
	return &ZerologHandler{
		BaseHandler: handler.NewBaseHandler(minLevel, opts...),
		logger:      l,
	}
}

// ToZerologLevel maps a monogo.Level to the corresponding zerolog.Level.
// High severity levels (CRITICAL, ALERT, EMERGENCY) map to zerolog.ErrorLevel
// to avoid invoking zerolog.FatalLevel which triggers os.Exit(1).
func ToZerologLevel(lvl monogo.Level) zerolog.Level {
	switch {
	case lvl < monogo.INFO:
		return zerolog.DebugLevel
	case lvl < monogo.WARNING:
		return zerolog.InfoLevel
	case lvl < monogo.ERROR:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}

// FromZerologLevel maps a zerolog.Level to the closest monogo.Level.
func FromZerologLevel(lvl zerolog.Level) monogo.Level {
	switch lvl {
	case zerolog.TraceLevel, zerolog.DebugLevel:
		return monogo.DEBUG
	case zerolog.InfoLevel:
		return monogo.INFO
	case zerolog.WarnLevel:
		return monogo.WARNING
	case zerolog.ErrorLevel:
		return monogo.ERROR
	case zerolog.FatalLevel:
		return monogo.EMERGENCY
	case zerolog.PanicLevel:
		return monogo.ALERT
	default:
		return monogo.INFO
	}
}

func (z *ZerologHandler) Handle(ctx context.Context, record monogo.Record) error {
	record = z.ProcessRecord(record)
	zLevel := ToZerologLevel(record.Level)

	event := z.logger.WithLevel(zLevel)
	if !event.Enabled() {
		return nil
	}

	if ctx != nil {
		event = event.Ctx(ctx)
	}

	if !record.Time.IsZero() {
		event = event.Time(zerolog.TimestampFieldName, record.Time)
	}

	if record.Level == monogo.NOTICE || record.Level >= monogo.CRITICAL {
		event = event.Str("severity", record.Level.String())
	}

	if record.Channel != "" {
		event = event.Str("channel", record.Channel)
	}

	if len(record.Context) > 0 {
		event = event.Fields(record.Context)
	}

	if len(record.Extra) > 0 {
		event = event.Fields(map[string]interface{}{"extra": record.Extra})
	}

	event.Msg(record.Message)
	return nil
}

func (z *ZerologHandler) Close() error {
	return nil
}
