package middleware_test

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/middleware"
	"github.com/githoober/monogo/processor"
)

func TestHTTPMiddleware_Basic(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, nil)

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok response"))
	})

	mw := middleware.HTTP(l)
	ts := httptest.NewServer(mw(handlerFunc))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/test-endpoint")
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	reqID := res.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Errorf("expected X-Request-ID header to be set")
	}

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(records))
	}
	if records[0].Message != "HTTP request handled" {
		t.Errorf("unexpected message: %q", records[0].Message)
	}
	if records[0].Level != monogo.INFO {
		t.Errorf("expected INFO level, got %v", records[0].Level)
	}
	if records[0].Context["status"] != http.StatusOK {
		t.Errorf("expected status 200, got %v", records[0].Context["status"])
	}
	if records[0].Context["request_id"] != reqID {
		t.Errorf("expected context request_id to match header, got %v", records[0].Context["request_id"])
	}
}

func TestHTTPMiddleware_ExistingRequestIDAndCustomOptions(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, nil)

	innerCalled := false
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	})

	mw := middleware.HTTP(l,
		middleware.WithRequestIDHeader("X-Trace-Id"),
		middleware.WithMessage("request finished"),
	)

	req := httptest.NewRequest(http.MethodPost, "/fail", nil)
	req.Header.Set("X-Trace-Id", "trace-custom-12345")
	w := httptest.NewRecorder()

	mw(handlerFunc).ServeHTTP(w, req)

	if !innerCalled {
		t.Errorf("expected inner handler to be called")
	}
	if w.Header().Get("X-Trace-Id") != "trace-custom-12345" {
		t.Errorf("expected preserved trace ID, got %q", w.Header().Get("X-Trace-Id"))
	}

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != monogo.ERROR {
		t.Errorf("expected status 500 to produce ERROR level, got %v", records[0].Level)
	}
	if records[0].Message != "request finished" {
		t.Errorf("expected custom message, got %q", records[0].Message)
	}
}

func TestHTTPMiddleware_CustomLevelFunc(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, nil)

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	mw := middleware.HTTP(l,
		middleware.WithLevelFunc(func(status int) monogo.Level {
			if status == http.StatusNotFound {
				return monogo.NOTICE
			}
			return monogo.INFO
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	mw(handlerFunc).ServeHTTP(w, req)

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Level != monogo.NOTICE {
		t.Errorf("expected custom level NOTICE, got %v", records[0].Level)
	}
}

func TestHTTPMiddleware_StatusCommitment(t *testing.T) {
	// Case A: WriteHeader(201) followed by WriteHeader(500) -> logs 201
	{
		testH := handler.NewTest(monogo.DEBUG)
		l := monogo.New("http", []monogo.Handler{testH}, nil)
		handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusInternalServerError)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		middleware.HTTP(l)(handlerFunc).ServeHTTP(w, req)

		records := testH.Records()
		if len(records) != 1 || records[0].Context["status"] != http.StatusCreated {
			t.Fatalf("expected logged status 201, got %v", records[0].Context["status"])
		}
	}

	// Case B: Write() implicitly commits 200, followed by WriteHeader(500) -> logs 200
	{
		testH := handler.NewTest(monogo.DEBUG)
		l := monogo.New("http", []monogo.Handler{testH}, nil)
		handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
			w.WriteHeader(http.StatusInternalServerError)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		middleware.HTTP(l)(handlerFunc).ServeHTTP(w, req)

		records := testH.Records()
		if len(records) != 1 || records[0].Context["status"] != http.StatusOK {
			t.Fatalf("expected logged status 200, got %v", records[0].Context["status"])
		}
	}

	// Case C: 1xx informational status (103) followed by final 200 -> logs 200
	{
		testH := handler.NewTest(monogo.DEBUG)
		l := monogo.New("http", []monogo.Handler{testH}, nil)
		handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(103) // Early Hints
			w.WriteHeader(http.StatusOK)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		middleware.HTTP(l)(handlerFunc).ServeHTTP(w, req)

		records := testH.Records()
		if len(records) != 1 || records[0].Context["status"] != http.StatusOK {
			t.Fatalf("expected logged status 200, got %v", records[0].Context["status"])
		}
	}
}

func TestHTTPMiddleware_PanicRecoveryAndLogging(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, nil)

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection explosion")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)

	var recovered interface{}
	func() {
		defer func() {
			recovered = recover()
		}()
		middleware.HTTP(l)(handlerFunc).ServeHTTP(w, req)
	}()

	if recovered == nil || recovered != "database connection explosion" {
		t.Fatalf("expected middleware to re-panic with original error, got: %v", recovered)
	}

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record logged on panic, got %d", len(records))
	}
	if records[0].Level != monogo.ERROR {
		t.Errorf("expected ERROR level on panic, got %v", records[0].Level)
	}
	if records[0].Context["status"] != http.StatusInternalServerError {
		t.Errorf("expected status 500 on panic without header commitment, got %v", records[0].Context["status"])
	}
	if records[0].Context["panic"] != "database connection explosion" {
		t.Errorf("expected panic field recorded, got %v", records[0].Context["panic"])
	}
}

type dummyFlusherHijacker struct {
	http.ResponseWriter
	flushed  bool
	hijacked bool
}

func (d *dummyFlusherHijacker) Flush() {
	d.flushed = true
}

func (d *dummyFlusherHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	d.hijacked = true
	return nil, nil, nil
}

func TestHTTPMiddleware_ResponseWriterInterfaces(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, nil)

	recorder := httptest.NewRecorder()
	dummy := &dummyFlusherHijacker{ResponseWriter: recorder}

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test Unwrap via http.ResponseController
		rc := http.NewResponseController(w)
		_ = rc.Flush()

		// Test direct Flusher type assertion
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Test direct Hijacker type assertion
		if h, ok := w.(http.Hijacker); ok {
			_, _, _ = h.Hijack()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	middleware.HTTP(l)(handlerFunc).ServeHTTP(dummy, req)

	if !dummy.flushed {
		t.Errorf("expected Flush() to reach underlying writer")
	}
	if !dummy.hijacked {
		t.Errorf("expected Hijack() to reach underlying writer")
	}
}

func TestHTTPMiddleware_TrustedProxies(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, []monogo.Processor{processor.Web()})

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.HTTP(l, middleware.WithTrustedProxies("10.0.0.1"))

	// Request 1: From trusted proxy 10.0.0.1 with XFF
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	req1.Header.Set("X-Forwarded-For", "203.0.113.50")
	mw(handlerFunc).ServeHTTP(httptest.NewRecorder(), req1)

	// Request 2: From untrusted client with spoofed XFF
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "198.51.100.1:1234"
	req2.Header.Set("X-Forwarded-For", "203.0.113.50")
	mw(handlerFunc).ServeHTTP(httptest.NewRecorder(), req2)

	records := testH.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Extra["ip"] != "203.0.113.50" {
		t.Errorf("expected trusted XFF IP 203.0.113.50, got %v", records[0].Extra["ip"])
	}
	if records[1].Extra["ip"] != "198.51.100.1" {
		t.Errorf("expected untrusted caller RemoteAddr 198.51.100.1, got %v", records[1].Extra["ip"])
	}
}

func TestHTTPMiddleware_WithTrustedProxyFunc(t *testing.T) {
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("http", []monogo.Handler{testH}, []monogo.Processor{processor.Web()})

	mw := middleware.HTTP(l, middleware.WithTrustedProxyFunc(func(ip net.IP) bool {
		return ip.IsLoopback()
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5432"
	req.Header.Set("X-Forwarded-For", "203.0.113.77")

	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(httptest.NewRecorder(), req)

	records := testH.Records()
	if len(records) != 1 || records[0].Extra["ip"] != "203.0.113.77" {
		t.Fatalf("expected trusted loopback proxy to resolve XFF IP, got: %v", records[0].Extra["ip"])
	}
}

