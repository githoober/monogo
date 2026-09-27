package monolog

import (
	"fmt"
	"strings"
)

type Level int

const (
	DEBUG Level = iota * 100 + 100
	INFO
	NOTICE
	WARNING
	ERROR
	CRITICAL
	ALERT
	EMERGENCY
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case NOTICE:
		return "NOTICE"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	case CRITICAL:
		return "CRITICAL"
	case ALERT:
		return "ALERT"
	case EMERGENCY:
		return "EMERGENCY"
	default:
		return fmt.Sprintf("LEVEL(%d)", l)
	}
}

func ParseLevel(lvl string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(lvl)) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "NOTICE":
		return NOTICE, nil
	case "WARNING", "WARN":
		return WARNING, nil
	case "ERROR", "ERR":
		return ERROR, nil
	case "CRITICAL", "CRIT":
		return CRITICAL, nil
	case "ALERT":
		return ALERT, nil
	case "EMERGENCY", "EMERG":
		return EMERGENCY, nil
	default:
		return DEBUG, fmt.Errorf("unknown log level: %s", lvl)
	}
}
