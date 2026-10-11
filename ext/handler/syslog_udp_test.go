package handler_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/formatter"
)

func TestSyslogUdp_SendAndReceive(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on UDP: %v", err)
	}
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)
	h := handler.NewSyslogUdp(addr.IP.String(), addr.Port, monogo.DEBUG)
	ctx := t.Context()

	rec := monogo.Record{
		Message: "syslog udp test message",
		Level:   monogo.WARNING,
		Channel: "network",
		Time:    time.Now().UTC(),
	}

	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	buf := make([]byte, 1024)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom UDP server failed: %v", err)
	}

	received := string(buf[:n])
	if !strings.Contains(received, "syslog udp test message") {
		t.Errorf("expected received payload to contain log message, got: %s", received)
	}
	if !strings.Contains(received, "network") {
		t.Errorf("expected received payload to contain channel, got: %s", received)
	}

	_ = h.Close(ctx)
}

func TestSyslogUdp_CustomFormatterAndBatch(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on UDP: %v", err)
	}
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)
	customFmt := formatter.NewSyslog("custom-app",
		formatter.WithSyslogFacility(formatter.FacilityLocal2),
	)

	h := handler.NewSyslogUdp(addr.IP.String(), addr.Port, monogo.INFO,
		handler.WithFormatter(customFmt),
	)
	ctx := t.Context()

	records := []monogo.Record{
		{Message: "batch item 1", Level: monogo.INFO, Time: time.Now().UTC()},
		{Message: "batch item 2", Level: monogo.ERROR, Time: time.Now().UTC()},
	}

	if err := h.HandleBatch(ctx, records); err != nil {
		t.Fatalf("HandleBatch failed: %v", err)
	}

	_ = h.Close(ctx)
}
