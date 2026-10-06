package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/processor"
)

// Option configures HTTP middleware behavior.
type Option func(*options)

type options struct {
	requestIDHeader string
	levelFunc       func(status int) monogo.Level
	message         string
}

func defaultOptions() options {
	return options{
		requestIDHeader: "X-Request-ID",
		levelFunc: func(status int) monogo.Level {
			switch {
			case status >= 500:
				return monogo.ERROR
			case status >= 400:
				return monogo.WARNING
			default:
				return monogo.INFO
			}
		},
		message: "HTTP request handled",
	}
}

// WithRequestIDHeader configures the HTTP header used to read/write the request ID (default: X-Request-ID).
func WithRequestIDHeader(header string) Option {
	return func(o *options) {
		o.requestIDHeader = header
	}
}

// WithLevelFunc configures a custom function that determines the log level from the HTTP status code.
func WithLevelFunc(fn func(status int) monogo.Level) Option {
	return func(o *options) {
		o.levelFunc = fn
	}
}

// WithMessage configures the log message written on request completion (default: "HTTP request handled").
func WithMessage(msg string) Option {
	return func(o *options) {
		o.message = msg
	}
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *statusCapturingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// HTTP returns a standard net/http middleware that logs requests and attaches ambient HTTP context metadata.
func HTTP(logger *monogo.Logger, opts ...Option) func(http.Handler) http.Handler {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get(o.requestIDHeader)
			if reqID == "" {
				reqID = generateRequestID()
			}
			w.Header().Set(o.requestIDHeader, reqID)

			ctx := processor.WithHTTPRequest(r.Context(), r)
			ctx = monogo.WithField(ctx, "request_id", reqID)
			r = r.WithContext(ctx)

			start := time.Now()
			sw := &statusCapturingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			lvl := o.levelFunc(sw.statusCode)

			_ = logger.Log(ctx, lvl, o.message, map[string]interface{}{
				"status":      sw.statusCode,
				"duration_ms": duration.Milliseconds(),
				"bytes":       sw.bytesWritten,
			})
		})
	}
}
