package handler

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
)

// Socket streams formatted log records over a network socket (TCP, UDP, Unix domain socket).
// It is modeled after PHP Monolog's SocketHandler.
type Socket struct {
	BaseHandler
	network      string
	address      string
	dialTimeout  time.Duration
	writeTimeout time.Duration
	dialer       func(ctx context.Context, network, address string) (net.Conn, error)

	mu   sync.Mutex
	conn net.Conn
}

var (
	_ monogo.Handler      = (*Socket)(nil)
	_ monogo.BatchHandler = (*Socket)(nil)
	_ monogo.Resettable   = (*Socket)(nil)
)

// NewSocket creates a Socket handler that writes formatted records to network and address at or above minLevel.
// Supported networks include "tcp", "udp", "unix", etc.
func NewSocket(network, address string, minLevel monogo.Level, opts ...Option) *Socket {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	if o.formatter == nil {
		o.formatter = formatter.NewLine("", "")
	}

	dialTimeout := 5 * time.Second
	if o.dialTimeout > 0 {
		dialTimeout = o.dialTimeout
	}

	writeTimeout := 5 * time.Second
	if o.writeTimeout > 0 {
		writeTimeout = o.writeTimeout
	}

	dialer := o.dialer
	if dialer == nil {
		dialer = func(ctx context.Context, netw, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, netw, addr)
		}
	}

	h := &Socket{
		BaseHandler:  NewBaseHandler(minLevel, opts...),
		network:      network,
		address:      address,
		dialTimeout:  dialTimeout,
		writeTimeout: writeTimeout,
		dialer:       dialer,
	}
	if h.formatter == nil {
		h.formatter = formatter.NewLine("", "")
	}
	return h
}

// connect ensures an active connection is established. Caller must hold s.mu.
func (s *Socket) connect(ctx context.Context) error {
	if s.conn != nil {
		return nil
	}
	dialCtx := ctx
	var cancel context.CancelFunc
	if s.dialTimeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, s.dialTimeout)
		defer cancel()
	}
	conn, err := s.dialer(dialCtx, s.network, s.address)
	if err != nil {
		return fmt.Errorf("monogo socket dial %s://%s failed: %w", s.network, s.address, err)
	}
	s.conn = conn
	return nil
}

// writeLocked writes bytes to socket with timeout. Attempts one reconnection on failure. Caller must hold s.mu.
func (s *Socket) writeLocked(ctx context.Context, p []byte) error {
	if err := s.connect(ctx); err != nil {
		return err
	}

	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}

	_, err := s.conn.Write(p)
	if err != nil {
		// Attempt one reconnect and retry
		_ = s.conn.Close()
		s.conn = nil
		if recErr := s.connect(ctx); recErr != nil {
			return fmt.Errorf("socket write error: %v, reconnect error: %w", err, recErr)
		}
		if s.writeTimeout > 0 {
			_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
		}
		_, err = s.conn.Write(p)
		if err != nil {
			_ = s.conn.Close()
			s.conn = nil
			return fmt.Errorf("socket write retry error: %w", err)
		}
	}
	return nil
}

// Handle processes, formats, and writes a log record to the socket.
func (s *Socket) Handle(ctx context.Context, record monogo.Record) error {
	if !s.IsHandling(ctx, record.Level) {
		return nil
	}

	record = s.ProcessRecord(record)
	formatted, err := s.Formatter().Format(record)
	if err != nil {
		return fmt.Errorf("socket format error: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(ctx, formatted)
}

// HandleBatch processes, formats, and writes a batch of records to the socket.
func (s *Socket) HandleBatch(ctx context.Context, records []monogo.Record) error {
	filtered := make([]monogo.Record, 0, len(records))
	for _, rec := range records {
		if s.IsHandling(ctx, rec.Level) {
			filtered = append(filtered, s.ProcessRecord(rec))
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	var formatted []byte
	if bf, ok := s.Formatter().(monogo.BatchFormatter); ok {
		var err error
		formatted, err = bf.FormatBatch(filtered)
		if err != nil {
			return fmt.Errorf("socket batch format error: %w", err)
		}
	} else {
		for _, rec := range filtered {
			b, err := s.Formatter().Format(rec)
			if err != nil {
				return fmt.Errorf("socket batch record format error: %w", err)
			}
			formatted = append(formatted, b...)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(ctx, formatted)
}

// Close closes the active socket connection.
func (s *Socket) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

// Reset resets per-handler processors and closes active socket connection to reconnect afresh.
func (s *Socket) Reset(ctx context.Context) error {
	var lastErr error
	if err := s.BaseHandler.Reset(ctx); err != nil {
		lastErr = err
	}
	if err := s.Close(ctx); err != nil {
		lastErr = err
	}
	return lastErr
}
