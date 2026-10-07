package processor_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

func TestWebProcessor_WithHTTPRequest(t *testing.T) {
	ctx := t.Context()
	req := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/users?page=2", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://example.com")
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")

	// 10.0.0.1 is configured as a trusted proxy
	ctx = processor.WithHTTPRequest(ctx, req, processor.WithTrustedProxies("10.0.0.1"))

	webProc := processor.Web()
	rec := monogo.Record{
		Message: "request processing",
		Level:   monogo.INFO,
		Context: map[string]interface{}{},
	}
	// Extract ambient fields into record context
	for k, v := range monogo.FromContext(ctx) {
		rec.Context[k] = v
	}

	rec = webProc.Process(rec)

	if rec.Extra["url"] != "https://api.example.com/v1/users?page=2" {
		t.Errorf("unexpected url: %v", rec.Extra["url"])
	}
	if rec.Extra["ip"] != "203.0.113.195" {
		t.Errorf("unexpected ip: %v", rec.Extra["ip"])
	}
	if rec.Extra["http_method"] != http.MethodGet {
		t.Errorf("unexpected http_method: %v", rec.Extra["http_method"])
	}
	if rec.Extra["server"] != "api.example.com" {
		t.Errorf("unexpected server: %v", rec.Extra["server"])
	}
	if rec.Extra["referrer"] != "https://example.com" {
		t.Errorf("unexpected referrer: %v", rec.Extra["referrer"])
	}
	if rec.Extra["user_agent"] != "Mozilla/5.0" {
		t.Errorf("unexpected user_agent: %v", rec.Extra["user_agent"])
	}
}

func TestWebProcessor_ClientIPResolution(t *testing.T) {
	// 1. Untrusted caller: XFF and X-Real-IP are ignored, defaults to RemoteAddr
	reqUntrusted := httptest.NewRequest(http.MethodPost, "/", nil)
	reqUntrusted.RemoteAddr = "198.51.100.55:1234"
	reqUntrusted.Header.Set("X-Forwarded-For", "1.2.3.4")
	reqUntrusted.Header.Set("X-Real-IP", "5.6.7.8")
	dataUntrusted := processor.ExtractHTTPRequestData(reqUntrusted)
	if dataUntrusted.IP != "198.51.100.55" {
		t.Errorf("expected RemoteAddr 198.51.100.55 when untrusted, got %s", dataUntrusted.IP)
	}

	// 2. Trusted proxy with XFF
	reqTrustedXFF := httptest.NewRequest(http.MethodGet, "/", nil)
	reqTrustedXFF.RemoteAddr = "10.0.0.2:1234"
	reqTrustedXFF.Header.Set("X-Forwarded-For", "203.0.113.195, 10.0.0.2")
	dataTrustedXFF := processor.ExtractHTTPRequestData(reqTrustedXFF, processor.WithTrustedProxies("10.0.0.0/8"))
	if dataTrustedXFF.IP != "203.0.113.195" {
		t.Errorf("expected trusted XFF IP 203.0.113.195, got %s", dataTrustedXFF.IP)
	}

	// 3. Trusted proxy with X-Real-IP
	reqTrustedXRI := httptest.NewRequest(http.MethodGet, "/", nil)
	reqTrustedXRI.RemoteAddr = "127.0.0.1:1234"
	reqTrustedXRI.Header.Set("X-Real-IP", "192.0.2.1")
	dataTrustedXRI := processor.ExtractHTTPRequestData(reqTrustedXRI, processor.WithTrustedProxies("127.0.0.1"))
	if dataTrustedXRI.IP != "192.0.2.1" {
		t.Errorf("expected trusted X-Real-IP 192.0.2.1, got %s", dataTrustedXRI.IP)
	}

	// 4. RemoteAddr unparseable hostport fallback
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "pipe"
	data3 := processor.ExtractHTTPRequestData(req3)
	if data3.IP != "pipe" {
		t.Errorf("expected IP pipe, got %s", data3.IP)
	}

	// 5. Nil request
	dataNil := processor.ExtractHTTPRequestData(nil)
	if dataNil.IP != "" {
		t.Errorf("expected empty IP for nil request, got %s", dataNil.IP)
	}
}

func TestWebProcessor_WithWebExtraKey(t *testing.T) {
	ctx := t.Context()
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	ctx = processor.WithHTTPRequest(ctx, req)

	webProc := processor.Web(processor.WithWebExtraKey("http"))
	rec := monogo.Record{
		Message: "file upload",
		Level:   monogo.INFO,
		Context: map[string]interface{}{},
	}
	for k, v := range monogo.FromContext(ctx) {
		rec.Context[k] = v
	}

	rec = webProc.Process(rec)

	nested, ok := rec.Extra["http"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['http'] map, got %T", rec.Extra["http"])
	}
	if nested["http_method"] != http.MethodPost {
		t.Errorf("expected POST, got %v", nested["http_method"])
	}
	if nested["url"] != "/upload" {
		t.Errorf("expected /upload, got %v", nested["url"])
	}
}

func TestWebProcessor_WebFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/resource/42", nil)
	proc := processor.WebFromRequest(req, processor.WithWebExtraKey("request"))

	rec := monogo.Record{Message: "delete op"}
	rec = proc.Process(rec)

	nested, ok := rec.Extra["request"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Extra['request'] map, got %T", rec.Extra["request"])
	}
	if nested["http_method"] != http.MethodDelete {
		t.Errorf("expected DELETE, got %v", nested["http_method"])
	}

	// Unnested
	procDirect := processor.WebFromRequest(req)
	recDirect := monogo.Record{Message: "delete direct"}
	recDirect = procDirect.Process(recDirect)
	if recDirect.Extra["http_method"] != http.MethodDelete {
		t.Errorf("expected direct DELETE, got %v", recDirect.Extra["http_method"])
	}
}

func TestWebProcessor_NilAndEmptySafeties(t *testing.T) {
	ctx := t.Context()
	ctxNil := processor.WithHTTPRequest(ctx, nil)
	if ctxNil != ctx {
		t.Errorf("expected identical context when req is nil")
	}

	proc := processor.Web()
	rec := monogo.Record{Message: "no http"}
	res := proc.Process(rec)
	if len(res.Extra) != 0 {
		t.Errorf("expected 0 extra fields when no http context present")
	}
}

func TestWebProcessor_WithTrustedProxyFunc(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.88")

	data := processor.ExtractHTTPRequestData(req, processor.WithTrustedProxyFunc(func(ip net.IP) bool {
		return ip.String() == "10.1.2.3"
	}))
	if data.IP != "203.0.113.88" {
		t.Errorf("expected 203.0.113.88, got %s", data.IP)
	}
}

func TestWebProcessor_WithWebTrustedProxies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Real-IP", "203.0.113.99")

	proc := processor.WebFromRequest(req, processor.WithWebTrustedProxies("192.168.1.1"))
	rec := proc(monogo.Record{Extra: map[string]interface{}{}})
	if rec.Extra["ip"] != "203.0.113.99" {
		t.Errorf("expected 203.0.113.99, got %v", rec.Extra["ip"])
	}
}
