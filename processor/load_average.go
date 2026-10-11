package processor

import (
	"os"
	"strconv"
	"strings"

	"github.com/githoober/monogo"
)

// LoadPeriod specifies which load average interval to record.
type LoadPeriod int

const (
	// Load1Minute records the 1-minute system load average.
	Load1Minute LoadPeriod = 1
	// Load5Minute records the 5-minute system load average.
	Load5Minute LoadPeriod = 5
	// Load15Minute records the 15-minute system load average.
	Load15Minute LoadPeriod = 15
	// LoadAll records all three (1m, 5m, 15m) intervals in a map.
	LoadAll LoadPeriod = 0
)

type loadConfig struct {
	period     LoadPeriod
	extraKey   string
	loadReader func() ([3]float64, error)
}

// LoadOption configures the LoadAverage processor.
type LoadOption func(*loadConfig)

// WithLoadPeriod configures the load average time period to record (Load1Minute, Load5Minute, Load15Minute, or LoadAll).
func WithLoadPeriod(period LoadPeriod) LoadOption {
	return func(c *loadConfig) {
		c.period = period
	}
}

// WithLoadExtraKey configures the key in Record.Extra where load average data is written (default: "load_average").
func WithLoadExtraKey(key string) LoadOption {
	return func(c *loadConfig) {
		if key != "" {
			c.extraKey = key
		}
	}
}

func defaultLoadReader() ([3]float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}, os.ErrInvalid
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err2 := strconv.ParseFloat(fields[1], 64)
	l15, err3 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return [3]float64{}, os.ErrInvalid
	}
	return [3]float64{l1, l5, l15}, nil
}

// LoadAverage creates a processor that injects system CPU load averages into Record.Extra["load_average"].
// Modeled after PHP Monolog's LoadAverageProcessor.
func LoadAverage(opts ...LoadOption) monogo.ProcessorFunc {
	cfg := loadConfig{
		period:     Load1Minute,
		extraKey:   "load_average",
		loadReader: defaultLoadReader,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return func(r monogo.Record) monogo.Record {
		loads, err := cfg.loadReader()
		if err != nil {
			return r
		}

		if r.Extra == nil {
			r.Extra = make(map[string]interface{})
		}

		switch cfg.period {
		case Load5Minute:
			r.Extra[cfg.extraKey] = loads[1]
		case Load15Minute:
			r.Extra[cfg.extraKey] = loads[2]
		case LoadAll:
			r.Extra[cfg.extraKey] = map[string]float64{
				"1m":  loads[0],
				"5m":  loads[1],
				"15m": loads[2],
			}
		default: // Load1Minute
			r.Extra[cfg.extraKey] = loads[0]
		}

		return r
	}
}
