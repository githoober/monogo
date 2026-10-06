package handler_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func TestSocketHandler_TCP(t *testing.T) {
	ctx := t.Context()

	// Start local TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on TCP: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			mu.Lock()
			received = append(received, scanner.Text())
			mu.Unlock()
		}
	}()

	sockH := handler.NewSocket("tcp", ln.Addr().String(), monogo.INFO,
		handler.WithFormatter(formatter.NewLine("%message%\n", "")),
		handler.WithWriteTimeout(2*time.Second),
		handler.WithDialTimeout(2*time.Second),
	)

	if err := sockH.Handle(ctx, monogo.Record{Message: "msg 1", Level: monogo.INFO}); err != nil {
		t.Fatalf("unexpected error on Handle: %v", err)
	}
	if err := sockH.Handle(ctx, monogo.Record{Message: "msg 2", Level: monogo.ERROR}); err != nil {
		t.Fatalf("unexpected error on Handle: %v", err)
	}

	// Close handler to flush and close connection
	if err := sockH.Close(ctx); err != nil {
		t.Fatalf("unexpected error on Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server to receive records")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(received), received)
	}
	if received[0] != "msg 1" || received[1] != "msg 2" {
		t.Errorf("unexpected received lines: %v", received)
	}
}

func TestSocketHandler_Batch(t *testing.T) {
	ctx := t.Context()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on TCP: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			mu.Lock()
			received = append(received, scanner.Text())
			mu.Unlock()
		}
	}()

	sockH := handler.NewSocket("tcp", ln.Addr().String(), monogo.DEBUG,
		handler.WithFormatter(formatter.NewLine("%message%\n", "")),
	)

	records := []monogo.Record{
		{Message: "batch line 1", Level: monogo.INFO},
		{Message: "batch line 2", Level: monogo.DEBUG},
	}

	if err := sockH.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected batch error: %v", err)
	}

	// Empty batch handling
	if err := sockH.HandleBatch(ctx, nil); err != nil {
		t.Fatalf("unexpected empty batch error: %v", err)
	}

	_ = sockH.Close(ctx)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for batch lines")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 batch lines, got %d: %v", len(received), received)
	}
	if received[0] != "batch line 1" || received[1] != "batch line 2" {
		t.Errorf("unexpected batch lines: %v", received)
	}
}

func TestSocketHandler_ReconnectOnFailure(t *testing.T) {
	ctx := t.Context()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var acceptedCount int
	var mu sync.Mutex

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			acceptedCount++
			count := acceptedCount
			mu.Unlock()

			if count == 1 {
				// Immediately close the first connection to force a reconnect on the client side
				_ = conn.Close()
			} else {
				// Keep subsequent connections open and read to EOF
				go func(c net.Conn) {
					_, _ = io.ReadAll(c)
					_ = c.Close()
				}(conn)
			}
		}
	}()

	sockH := handler.NewSocket("tcp", ln.Addr().String(), monogo.DEBUG,
		handler.WithFormatter(formatter.NewLine("%message%\n", "")),
		handler.WithWriteTimeout(2*time.Second),
	)

	// First write might succeed in connecting, but then if connection closed, next write reconnects
	_ = sockH.Handle(ctx, monogo.Record{Message: "hello 1", Level: monogo.INFO})

	// Wait briefly to ensure server has closed first connection
	time.Sleep(50 * time.Millisecond)

	// Second write should trigger reconnection and succeed
	err = sockH.Handle(ctx, monogo.Record{Message: "hello 2", Level: monogo.INFO})
	if err != nil {
		t.Fatalf("expected successful write with reconnect, got: %v", err)
	}

	_ = sockH.Close(ctx)
}

func TestSocketHandler_CustomDialerAndReset(t *testing.T) {
	ctx := t.Context()
	serverConn, clientConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()

	dialed := 0
	sockH := handler.NewSocket("custom", "pipe", monogo.DEBUG,
		handler.WithDialer(func(_ context.Context, _, _ string) (net.Conn, error) {
			dialed++
			return clientConn, nil
		}),
		handler.WithFormatter(formatter.NewLine("%message%\n", "")),
	)

	go func() {
		buf := make([]byte, 1024)
		_, _ = serverConn.Read(buf)
	}()

	if err := sockH.Handle(ctx, monogo.Record{Message: "pipe msg", Level: monogo.INFO}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialed != 1 {
		t.Errorf("expected 1 dial, got %d", dialed)
	}

	// Reset closes connection so next write redials
	if err := sockH.Reset(ctx); err != nil {
		t.Fatalf("unexpected error on reset: %v", err)
	}

	// Close on already closed connection
	if err := sockH.Close(ctx); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}
}

func TestSocketHandler_DialError(t *testing.T) {
	ctx := t.Context()
	sockH := handler.NewSocket("custom", "fail", monogo.DEBUG,
		handler.WithDialer(func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		}),
	)

	err := sockH.Handle(ctx, monogo.Record{Message: "fail", Level: monogo.INFO})
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected dial error, got %v", err)
	}
}

type mockErrFormatter struct{}

func (m mockErrFormatter) Format(_ monogo.Record) ([]byte, error) {
	return nil, errors.New("format failed")
}

type mockBatchErrFormatter struct{}

func (m mockBatchErrFormatter) Format(_ monogo.Record) ([]byte, error) {
	return nil, nil
}

func (m mockBatchErrFormatter) FormatBatch(_ []monogo.Record) ([]byte, error) {
	return nil, errors.New("batch format failed")
}

type errResettableProcessor struct{}

func (e errResettableProcessor) Process(rec monogo.Record) monogo.Record {
	return rec
}

func (e errResettableProcessor) Reset(_ context.Context) error {
	return errors.New("reset failed")
}

func TestSocketHandler_UDP(t *testing.T) {
	ctx := t.Context()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on udp: %v", err)
	}
	defer func() { _ = pc.Close() }()

	sockH := handler.NewSocket("udp", pc.LocalAddr().String(), monogo.INFO,
		handler.WithFormatter(formatter.NewLine("%message%", "")),
	)

	if err := sockH.Handle(ctx, monogo.Record{Message: "udp hello", Level: monogo.INFO}); err != nil {
		t.Fatalf("unexpected UDP Handle error: %v", err)
	}

	buf := make([]byte, 1024)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("failed to read UDP packet: %v", err)
	}
	if string(buf[:n]) != "udp hello" {
		t.Errorf("expected 'udp hello', got %q", string(buf[:n]))
	}

	_ = sockH.Close(ctx)
}

func TestSocketHandler_LevelFiltering(t *testing.T) {
	ctx := t.Context()
	sockH := handler.NewSocket("tcp", "127.0.0.1:0", monogo.ERROR)

	if err := sockH.Handle(ctx, monogo.Record{Message: "info msg", Level: monogo.INFO}); err != nil {
		t.Fatalf("expected nil for filtered Handle, got %v", err)
	}

	if err := sockH.HandleBatch(ctx, []monogo.Record{{Message: "debug msg", Level: monogo.DEBUG}}); err != nil {
		t.Fatalf("expected nil for filtered HandleBatch, got %v", err)
	}
}

func TestSocketHandler_BatchFormatter(t *testing.T) {
	ctx := t.Context()
	serverConn, clientConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()

	sockH := handler.NewSocket("custom", "pipe", monogo.DEBUG,
		handler.WithDialer(func(_ context.Context, _, _ string) (net.Conn, error) {
			return clientConn, nil
		}),
		handler.WithFormatter(formatter.NewJSON("")),
	)

	var readBuf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 2048)
		n, _ := serverConn.Read(buf)
		readBuf.Write(buf[:n])
	}()

	records := []monogo.Record{
		{Message: "batch json 1", Level: monogo.INFO},
		{Message: "batch json 2", Level: monogo.ERROR},
	}
	if err := sockH.HandleBatch(ctx, records); err != nil {
		t.Fatalf("unexpected HandleBatch error: %v", err)
	}
	_ = sockH.Close(ctx)

	<-done
	if !strings.Contains(readBuf.String(), "batch json 1") || !strings.Contains(readBuf.String(), "batch json 2") {
		t.Errorf("unexpected batch output: %s", readBuf.String())
	}
}

func TestSocketHandler_FormatterErrors(t *testing.T) {
	ctx := t.Context()
	sockH := handler.NewSocket("tcp", "127.0.0.1:12345", monogo.DEBUG,
		handler.WithFormatter(mockErrFormatter{}),
	)

	if err := sockH.Handle(ctx, monogo.Record{Level: monogo.INFO}); err == nil {
		t.Error("expected error from mockErrFormatter in Handle")
	}

	if err := sockH.HandleBatch(ctx, []monogo.Record{{Level: monogo.INFO}}); err == nil {
		t.Error("expected error from mockErrFormatter in HandleBatch (non-batch formatter)")
	}

	batchSockH := handler.NewSocket("tcp", "127.0.0.1:12345", monogo.DEBUG,
		handler.WithFormatter(mockBatchErrFormatter{}),
	)
	if err := batchSockH.HandleBatch(ctx, []monogo.Record{{Level: monogo.INFO}}); err == nil {
		t.Error("expected error from mockBatchErrFormatter in HandleBatch")
	}
}

type failingConn struct {
	net.Conn
	failWrite bool
}

func (f *failingConn) Write(b []byte) (n int, err error) {
	if f.failWrite {
		return 0, errors.New("write broken")
	}
	return len(b), nil
}

func (f *failingConn) Close() error {
	return nil
}

func (f *failingConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

func TestSocketHandler_ReconnectFails(t *testing.T) {
	ctx := t.Context()
	dialCount := 0

	sockH := handler.NewSocket("custom", "failing", monogo.DEBUG,
		handler.WithDialer(func(_ context.Context, _, _ string) (net.Conn, error) {
			dialCount++
			if dialCount == 1 {
				return &failingConn{failWrite: true}, nil
			}
			return nil, errors.New("reconnect dial failed")
		}),
	)

	err := sockH.Handle(ctx, monogo.Record{Level: monogo.INFO, Message: "test"})
	if err == nil || !strings.Contains(err.Error(), "reconnect dial failed") {
		t.Errorf("expected reconnect dial failed error, got: %v", err)
	}
}

func TestSocketHandler_RetryWriteFails(t *testing.T) {
	ctx := t.Context()

	sockH := handler.NewSocket("custom", "failing", monogo.DEBUG,
		handler.WithDialer(func(_ context.Context, _, _ string) (net.Conn, error) {
			return &failingConn{failWrite: true}, nil
		}),
	)

	err := sockH.Handle(ctx, monogo.Record{Level: monogo.INFO, Message: "test"})
	if err == nil || !strings.Contains(err.Error(), "socket write retry error") {
		t.Errorf("expected socket write retry error, got: %v", err)
	}
}

func TestSocketHandler_ResetProcessorError(t *testing.T) {
	ctx := t.Context()

	sockH := handler.NewSocket("custom", "dummy", monogo.DEBUG,
		handler.WithProcessor(errResettableProcessor{}),
	)

	err := sockH.Reset(ctx)
	if err == nil || !strings.Contains(err.Error(), "reset failed") {
		t.Errorf("expected reset error, got: %v", err)
	}
}
