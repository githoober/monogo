package zerologadapter

import (
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

func (z *ZerologHandler) Handle(record monogo.Record) error {
	zLevel := mapMonogoToZerologLevel(record.Level)

	event := z.logger.WithLevel(zLevel)
	if !event.Enabled() {
		return nil
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

func mapMonogoToZerologLevel(lvl monogo.Level) zerolog.Level {
	switch {
	case lvl < monogo.INFO:
		return zerolog.DebugLevel
	case lvl < monogo.WARNING:
		return zerolog.InfoLevel
	case lvl < monogo.ERROR:
		return zerolog.WarnLevel
	case lvl < monogo.CRITICAL:
		return zerolog.ErrorLevel
	default:
		return zerolog.FatalLevel
	}
}
