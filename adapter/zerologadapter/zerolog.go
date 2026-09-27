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

func NewZerologHandler(l zerolog.Logger, minLevel monolog.Level) *ZerologHandler {
	return &ZerologHandler{
		BaseHandler: handler.NewBaseHandler(minLevel, true),
		logger:      l,
	}
}

func (z *ZerologHandler) Handle(record monolog.Record) error {
	zLevel := mapMonologToZerologLevel(record.Level)

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

func mapMonologToZerologLevel(lvl monolog.Level) zerolog.Level {
	switch {
	case lvl < monolog.INFO:
		return zerolog.DebugLevel
	case lvl < monolog.WARNING:
		return zerolog.InfoLevel
	case lvl < monolog.ERROR:
		return zerolog.WarnLevel
	case lvl < monolog.CRITICAL:
		return zerolog.ErrorLevel
	default:
		return zerolog.FatalLevel
	}
}
