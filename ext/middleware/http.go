package middleware

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
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

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// Unwrap exposes the underlying ResponseWriter for http.ResponseController support.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}

	// 1xx status codes (except 101 Switching Protocols) are informational
	// intermediate responses and do not commit the final response header in net/http.
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}

	w.statusCode = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *responseWriter) flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriter) hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (w *responseWriter) push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *responseWriter) readFrom(src io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		w.bytesWritten += n
		return n, err
	}
	return io.Copy(struct{ io.Writer }{w}, src)
}

// Capability-specific wrapper variants ensuring optional ResponseWriter interfaces
// (http.Flusher, http.Hijacker, http.Pusher, io.ReaderFrom) are exposed only when the
// wrapped ResponseWriter implements them.

type wrapF struct{ *responseWriter }

func (w *wrapF) Flush() { w.flush() }

type wrapH struct{ *responseWriter }

func (w *wrapH) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }

type wrapP struct{ *responseWriter }

func (w *wrapP) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }

type wrapR struct{ *responseWriter }

func (w *wrapR) ReadFrom(src io.Reader) (int64, error) { return w.readFrom(src) }

type wrapFH struct{ *responseWriter }

func (w *wrapFH) Flush()                                               { w.flush() }
func (w *wrapFH) Hijack() (net.Conn, *bufio.ReadWriter, error)        { return w.hijack() }

type wrapFP struct{ *responseWriter }

func (w *wrapFP) Flush()                                          { w.flush() }
func (w *wrapFP) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }

type wrapFR struct{ *responseWriter }

func (w *wrapFR) Flush()                               { w.flush() }
func (w *wrapFR) ReadFrom(src io.Reader) (int64, error) { return w.readFrom(src) }

type wrapHP struct{ *responseWriter }

func (w *wrapHP) Hijack() (net.Conn, *bufio.ReadWriter, error)        { return w.hijack() }
func (w *wrapHP) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }

type wrapHR struct{ *responseWriter }

func (w *wrapHR) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }
func (w *wrapHR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

type wrapPR struct{ *responseWriter }

func (w *wrapPR) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }
func (w *wrapPR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

type wrapFHP struct{ *responseWriter }

func (w *wrapFHP) Flush()                                          { w.flush() }
func (w *wrapFHP) Hijack() (net.Conn, *bufio.ReadWriter, error)        { return w.hijack() }
func (w *wrapFHP) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }

type wrapFHR struct{ *responseWriter }

func (w *wrapFHR) Flush()                                       { w.flush() }
func (w *wrapFHR) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }
func (w *wrapFHR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

type wrapFPR struct{ *responseWriter }

func (w *wrapFPR) Flush()                                          { w.flush() }
func (w *wrapFPR) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }
func (w *wrapFPR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

type wrapHPR struct{ *responseWriter }

func (w *wrapHPR) Hijack() (net.Conn, *bufio.ReadWriter, error)        { return w.hijack() }
func (w *wrapHPR) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }
func (w *wrapHPR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

type wrapFHPR struct{ *responseWriter }

func (w *wrapFHPR) Flush()                                          { w.flush() }
func (w *wrapFHPR) Hijack() (net.Conn, *bufio.ReadWriter, error)        { return w.hijack() }
func (w *wrapFHPR) Push(target string, opts *http.PushOptions) error { return w.push(target, opts) }
func (w *wrapFHPR) ReadFrom(src io.Reader) (int64, error)         { return w.readFrom(src) }

func wrapResponseWriter(w http.ResponseWriter) (http.ResponseWriter, *responseWriter) {
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	_, flusher := w.(http.Flusher)
	_, hijacker := w.(http.Hijacker)
	_, pusher := w.(http.Pusher)
	_, readerFrom := w.(io.ReaderFrom)

	mask := 0
	if flusher {
		mask |= 1
	}
	if hijacker {
		mask |= 2
	}
	if pusher {
		mask |= 4
	}
	if readerFrom {
		mask |= 8
	}

	switch mask {
	case 1:
		return &wrapF{rw}, rw
	case 2:
		return &wrapH{rw}, rw
	case 3:
		return &wrapFH{rw}, rw
	case 4:
		return &wrapP{rw}, rw
	case 5:
		return &wrapFP{rw}, rw
	case 6:
		return &wrapHP{rw}, rw
	case 7:
		return &wrapFHP{rw}, rw
	case 8:
		return &wrapR{rw}, rw
	case 9:
		return &wrapFR{rw}, rw
	case 10:
		return &wrapHR{rw}, rw
	case 11:
		return &wrapFHR{rw}, rw
	case 12:
		return &wrapPR{rw}, rw
	case 13:
		return &wrapFPR{rw}, rw
	case 14:
		return &wrapHPR{rw}, rw
	case 15:
		return &wrapFHPR{rw}, rw
	default:
		return rw, rw
	}
}

var requestIDCounter uint64

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		seq := atomic.AddUint64(&requestIDCounter, 1)
		return fmt.Sprintf("req-%x-%016x", time.Now().UnixNano(), seq)
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
			wrapped, sw := wrapResponseWriter(w)

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

			next.ServeHTTP(wrapped, r)
			panicked = false
		})
	}
}
