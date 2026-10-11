package formatter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

func TestSyslog_Format(t *testing.T) {
	fmt := formatter.NewSyslog("payment-service",
		formatter.WithSyslogFacility(formatter.FacilityLocal0), // 16
		formatter.WithSyslogHostname("host.local"),
	)

	// Facility Local0 is 16. Severity for ERROR is 3.
	// Priority = (16 * 8) + 3 = 131.
	rec := monogo.Record{
		Message: "payment gateway timeout",
		Level:   monogo.ERROR,
		Channel: "billing",
		Time:    time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
		Context: map[string]interface{}{"retry": 3},
	}

	out, err := fmt.Format(rec)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	line := string(out)
	if !strings.HasPrefix(line, "<131>1 ") {
		t.Errorf("expected priority <131>1, got: %s", line)
	}
	if !strings.Contains(line, "host.local payment-service") {
		t.Errorf("expected host and app name in syslog line, got: %s", line)
	}
	if !strings.Contains(line, "payment gateway timeout") {
		t.Errorf("expected message in syslog line, got: %s", line)
	}
}

func TestSyslog_SeveritiesAndBatch(t *testing.T) {
	fmt := formatter.NewSyslog("app")

	levels := []monogo.Level{
		monogo.EMERGENCY, // 0
		monogo.ALERT,     // 1
		monogo.CRITICAL,  // 2
		monogo.ERROR,     // 3
		monogo.WARNING,   // 4
		monogo.NOTICE,    // 5
		monogo.INFO,      // 6
		monogo.DEBUG,     // 7
	}

	var records []monogo.Record
	for _, lvl := range levels {
		records = append(records, monogo.Record{
			Message: "level test",
			Level:   lvl,
			Time:    time.Now().UTC(),
		})
	}

	batchOut, err := fmt.FormatBatch(records)
	if err != nil {
		t.Fatalf("FormatBatch failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(batchOut)), "\n")
	if len(lines) != len(levels) {
		t.Fatalf("expected %d lines, got %d", len(levels), len(lines))
	}
}

func TestSyslog_Options(t *testing.T) {
	fmt := formatter.NewSyslog("initial-app",
		formatter.WithSyslogAppName("custom-app"),
		formatter.WithSyslogDateFormat(time.RFC3339),
	)

	rec := monogo.Record{
		Message: "test options",
		Level:   monogo.INFO,
		Time:    time.Date(2026, 10, 10, 12, 34, 56, 0, time.UTC),
	}

	out, err := fmt.Format(rec)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	line := string(out)
	if !strings.Contains(line, "custom-app") {
		t.Errorf("expected custom-app in syslog output, got: %s", line)
	}
	if !strings.Contains(line, "2026-10-10T12:34:56Z") {
		t.Errorf("expected RFC3339 formatted timestamp, got: %s", line)
	}
}
