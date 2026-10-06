package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/middleware"
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
