package middleware

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
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
	reqOptions      []processor.RequestOption
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

// WithTrustedProxies configures trusted proxy IP addresses or CIDR blocks for client IP extraction.
func WithTrustedProxies(proxies ...string) Option {
	return func(o *options) {
		o.reqOptions = append(o.reqOptions, processor.WithTrustedProxies(proxies...))
	}
}

// WithTrustedProxyFunc configures a custom predicate function for trusted proxy validation.
func WithTrustedProxyFunc(fn func(peerIP net.IP) bool) Option {
	return func(o *options) {
		o.reqOptions = append(o.reqOptions, processor.WithTrustedProxyFunc(fn))
	}
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// Unwrap exposes the underlying ResponseWriter for http.ResponseController support.
func (w *statusCapturingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusCapturingResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}

	w.statusCode = code
	// 1xx status codes (except 101 Switching Protocols) are informational
	// intermediate responses and do not commit the final response header in net/http.
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}

	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *statusCapturingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *statusCapturingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *statusCapturingResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		n, err := rf.ReadFrom(src)
		w.bytesWritten += n
		return n, err
	}
	return io.Copy(struct{ io.Writer }{w}, src)
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

			ctx := processor.WithHTTPRequest(r.Context(), r, o.reqOptions...)
			ctx = monogo.WithField(ctx, "request_id", reqID)
			r = r.WithContext(ctx)

			start := time.Now()
			sw := &statusCapturingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			panicked := true
			defer func() {
				duration := time.Since(start)
				if panicked {
					rErr := recover()
					status := sw.statusCode
					if !sw.wroteHeader {
						status = http.StatusInternalServerError
					}
					fields := map[string]interface{}{
						"status":      status,
						"duration_ms": duration.Milliseconds(),
						"bytes":       sw.bytesWritten,
					}
					if rErr != nil {
						fields["panic"] = fmt.Sprint(rErr)
					}
					_ = logger.Log(ctx, monogo.ERROR, o.message, fields)
					panic(rErr)
				}

				lvl := o.levelFunc(sw.statusCode)
				_ = logger.Log(ctx, lvl, o.message, map[string]interface{}{
					"status":      sw.statusCode,
					"duration_ms": duration.Milliseconds(),
					"bytes":       sw.bytesWritten,
				})
			}()

			next.ServeHTTP(sw, r)
			panicked = false
		})
	}
}
