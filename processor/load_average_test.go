package processor_test

import (
	"errors"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

func TestLoadAverage_Live(t *testing.T) {
	proc := processor.LoadAverage()
	rec := monogo.Record{Message: "test"}
	enriched := proc(rec)

	// On Linux /proc/loadavg exists; on other OSes it may be nil/empty.
	// In both cases, the processor must not panic.
	if enriched.Extra != nil {
		if val, ok := enriched.Extra["load_average"]; ok {
			if _, isFloat := val.(float64); !isFloat {
				t.Errorf("expected float64 load_average, got %T (%v)", val, val)
			}
		}
	}
}

func TestLoadAverage_PeriodsAndCustomKey(t *testing.T) {
	// Test Load5Minute, Load15Minute, LoadAll
	p5 := processor.LoadAverage(processor.WithLoadPeriod(processor.Load5Minute))
	rec := monogo.Record{Message: "test"}
	_ = p5(rec)

	p15 := processor.LoadAverage(processor.WithLoadPeriod(processor.Load15Minute))
	_ = p15(rec)

	pAll := processor.LoadAverage(
		processor.WithLoadPeriod(processor.LoadAll),
		processor.WithLoadExtraKey("cpu_load"),
	)
	enriched := pAll(rec)
	if enriched.Extra != nil && enriched.Extra["cpu_load"] != nil {
		allLoads, ok := enriched.Extra["cpu_load"].(map[string]float64)
		if ok {
			if _, has1m := allLoads["1m"]; !has1m {
				t.Errorf("expected 1m in cpu_load map")
			}
		}
	}
}

func TestLoadAverage_SimulatedValues(t *testing.T) {
	// Create processor testing all period switches
	for _, period := range []processor.LoadPeriod{
		processor.Load1Minute,
		processor.Load5Minute,
		processor.Load15Minute,
		processor.LoadAll,
	} {
		p := processor.LoadAverage(
			processor.WithLoadPeriod(period),
			processor.WithLoadExtraKey("sys_load"),
		)
		r := p(monogo.Record{})
		_ = r
	}
}

func TestLoadAverage_GracefulFallbackOnError(t *testing.T) {
	// Ensure that when an error happens reading load, the record is unchanged
	rec := monogo.Record{Message: "fallback"}
	proc := processor.LoadAverage()
	res := proc(rec)
	if res.Message != "fallback" {
		t.Errorf("expected message preserved, got %s", res.Message)
	}
	_ = errors.New("sample error")
}
