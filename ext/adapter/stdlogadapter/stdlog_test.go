package stdlogadapter_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/adapter/stdlogadapter"
	"github.com/githoober/monogo/handler"
)

type stdlogContextKey string

const testCtxKey stdlogContextKey = "trace_id"

func TestWriter_Basic(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("app", []monogo.Handler{testH}, nil)

	w := stdlogadapter.NewWriter(ctx, l, monogo.INFO)

	n, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len("hello world\n") {
		t.Errorf("expected n=%d, got %d", len("hello world\n"), n)
	}

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Message != "hello world" {
		t.Errorf("expected trimmed message 'hello world', got %q", records[0].Message)
	}
	if records[0].Level != monogo.INFO {
		t.Errorf("expected level INFO, got %v", records[0].Level)
	}
}

func TestWriter_EmptyWrite(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("app", []monogo.Handler{testH}, nil)

	w := stdlogadapter.NewWriter(ctx, l, monogo.INFO)

	n, err := w.Write([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if len(testH.Records()) != 0 {
		t.Errorf("expected 0 records, got %d", len(testH.Records()))
	}
}

func TestWriter_WithContextFunc(t *testing.T) {
	baseCtx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("app", []monogo.Handler{testH}, nil)

	dynamicCtx := context.WithValue(baseCtx, testCtxKey, "trace-999")

	w := stdlogadapter.NewWriter(baseCtx, l, monogo.WARNING,
		stdlogadapter.WithContextFunc(func() context.Context {
			return dynamicCtx
		}),
	)

	_, err := w.Write([]byte("warning event\r\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := testH.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Message != "warning event" {
		t.Errorf("expected trimmed message 'warning event', got %q", records[0].Message)
	}
	if records[0].Level != monogo.WARNING {
		t.Errorf("expected level WARNING, got %v", records[0].Level)
	}
}

func TestStdLogger_Integration(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("server", []monogo.Handler{testH}, nil)

	stdLog := stdlogadapter.NewStdLogger(ctx, l, monogo.ERROR, "[prefix] ", 0)

	stdLog.Print("error log message")
	stdLog.Println("second error log message")
	stdLog.Printf("formatted error: %d", 404)

	records := testH.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	if !strings.HasPrefix(records[0].Message, "[prefix] error log message") {
		t.Errorf("unexpected record 0 message: %q", records[0].Message)
	}
	if !strings.HasPrefix(records[1].Message, "[prefix] second error log message") {
		t.Errorf("unexpected record 1 message: %q", records[1].Message)
	}
	if !strings.HasPrefix(records[2].Message, "[prefix] formatted error: 404") {
		t.Errorf("unexpected record 2 message: %q", records[2].Message)
	}
	for i, r := range records {
		if r.Level != monogo.ERROR {
			t.Errorf("expected record %d to have ERROR level, got %v", i, r.Level)
		}
	}
}

type errorThrowingHandler struct {
	err error
}

func (e *errorThrowingHandler) IsHandling(_ context.Context, _ monogo.Level) bool { return true }
func (e *errorThrowingHandler) Handle(_ context.Context, _ monogo.Record) error    { return e.err }
func (e *errorThrowingHandler) Close(_ context.Context) error                     { return nil }

func TestWriter_ErrorPropagation(t *testing.T) {
	ctx := t.Context()
	errH := &errorThrowingHandler{err: errors.New("backend sink error")}
	l := monogo.New("app", []monogo.Handler{errH}, nil)

	w := stdlogadapter.NewWriter(ctx, l, monogo.INFO)
	_, err := w.Write([]byte("test\n"))
	if err == nil {
		t.Errorf("expected error from Write, got nil")
	}
}

func TestWriter_LineFraming(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("app", []monogo.Handler{testH}, nil)

	w := stdlogadapter.NewWriter(ctx, l, monogo.INFO)

	// Single write with multiple lines
	_, err := w.Write([]byte("first\nsecond\r\nthird\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Partial writes split across multiple calls
	_, err = w.Write([]byte("partial "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = w.Write([]byte("line\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := testH.Records()
	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}
	expected := []string{"first", "second", "third", "partial line"}
	for i, exp := range expected {
		if records[i].Message != exp {
			t.Errorf("record %d: expected %q, got %q", i, exp, records[i].Message)
		}
	}
}

func TestWriter_FlushAndClose(t *testing.T) {
	ctx := t.Context()
	testH := handler.NewTest(monogo.DEBUG)
	l := monogo.New("app", []monogo.Handler{testH}, nil)

	w := stdlogadapter.NewWriter(ctx, l, monogo.INFO)

	// Write without newline
	_, _ = w.Write([]byte("unterminated line"))
	if len(testH.Records()) != 0 {
		t.Errorf("expected 0 records before flush, got %d", len(testH.Records()))
	}

	// Flush commits the pending buffer
	if err := w.Flush(); err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}
	if len(testH.Records()) != 1 || testH.Records()[0].Message != "unterminated line" {
		t.Fatalf("expected 1 record after flush, got %v", testH.Records())
	}

	// Another partial write, closed
	_, _ = w.Write([]byte("another line"))
	if err := w.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if len(testH.Records()) != 2 || testH.Records()[1].Message != "another line" {
		t.Fatalf("expected 2 records after close, got %v", testH.Records())
	}

	// Flush on empty buffer is a no-op
	if err := w.Flush(); err != nil {
		t.Fatalf("unexpected flush error on empty: %v", err)
	}
}
