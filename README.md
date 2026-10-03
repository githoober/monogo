# Monogo

A flexible, channel-based generic structured logging library for Go inspired by PHP's Monolog. The core library is completely generic and decoupled from specific logging frameworks, allowing pluggable backend adapters like Go standard library `log/slog` and third-party loggers like `zerolog`.

## Features

- **Generic & Backend-Agnostic**: Core Monogo logger operates through generic `Handler`, `Processor`, and `Formatter` interfaces without hard dependencies on any specific backend.
- **Ambient Context Values**: Attach contextual fields (e.g., request ID, tenant ID, trace ID) to Go's `context.Context` using `monogo.WithContext` / `monogo.WithField`. These fields are automatically extracted and merged into log records on all log methods.
- **RFC 5424 / Monolog Log Levels**: `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY`.
- **Channel Support**: Easily categorize logs by channels (e.g. `app`, `auth`, `database`).
- **Handlers**: Stream, RotatingFile, Filter, Group, Buffer, FingersCrossed, Test, Null.
- **Per-Handler Processors**: Dedicated processor pipelines on individual handlers (`handler.WithProcessor(...)`) with copy-on-write record isolation to prevent mutation leakage across handlers.
- **Handler Bubbling Control**: Stop record propagation down the handler stack via `handler.WithBubble(false)` and the `monogo.Bubbler` interface.
- **First-Class Batch Processing**: Native `HandleBatch` and `FormatBatch` contracts across handlers and formatters for atomic, single-write flushing from buffering handlers (`Buffer`, `FingersCrossed`).
- **Processors**: Enriched logging metadata (Caller, Hostname, Memory stats, Tags, Unique request ID/UID).
- **Formatters**: Line, JSON (with NDJSON and JSON Array batch modes).
- **Backend Integrations**:
  - `slog` Backend & Bridge (use Monogo as backend for `slog`, or use `slog` as backend handler for Monogo).
  - `zerolog` Backend (use `zerolog` as a Monogo output handler).

## Installation

```bash
go get github.com/githoober/monogo
```

## Quick Start & Ambient Context

```go
package main

import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func main() {
	// Create a stream handler writing to stdout
	stdoutHandler := handler.NewStream(os.Stdout, monogo.INFO)

	// Create logger
	logger := monogo.New("main", []monogo.Handler{stdoutHandler}, []monogo.Processor{
		processor.Hostname(),
		processor.UID(),
	})

	// Set ambient fields in Go context
	ctx := context.Background()
	ctx = monogo.WithField(ctx, "request_id", "req-12345")
	ctx = monogo.WithContext(ctx, map[string]interface{}{
		"tenant": "acme-corp",
	})

	// Log messages with context; ambient context values are automatically included
	logger.Info(ctx, "User logged in", map[string]interface{}{"user_id": 42})
	logger.Warning(ctx, "Rate limit approaching", map[string]interface{}{"ip": "127.0.0.1"})
}
```

## Log File Rotation (RotatingFile)

Monogo provides a RotatingFile handler powered by lumberjack for automatic log file rotation based on file size, backup retention count, age, and optional compression:

```go
package main

import (
	"context"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func main() {
	ctx := context.Background()

	// Create a rotating file handler (rotates when log reaches 10MB, keeps 5 backups, retains for 30 days, compresses, formatted as JSON)
	rotHandler := handler.NewRotatingFile("app.log", monogo.DEBUG,
		handler.WithMaxSize(10),
		handler.WithMaxBackups(5),
		handler.WithMaxAge(30),
		handler.WithCompress(true),
		handler.WithFormatter(formatter.NewJSON("")),
	)
	defer rotHandler.Close(ctx)

	logger := monogo.New("app", []monogo.Handler{rotHandler}, nil)
	logger.Info(ctx, "App initialized with rolling log files")
}
```

## FingersCrossed Handler

The FingersCrossed handler buffers all low-level logs (such as DEBUG or INFO) silently and only flushes the entire buffer to a nested handler when a log record meets a specific action level (such as ERROR). Once triggered, it stays activated for subsequent logs.

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

// Create a stream output target
streamHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

// Wrap with FingersCrossed: buffers until ERROR level is triggered (buffer size up to 100 entries)
fcHandler := handler.NewFingersCrossed(streamHandler, monogo.ERROR, 100)

logger := monogo.New("app", []monogo.Handler{fcHandler}, nil)

logger.Debug(ctx, "Step 1 initialized") // Buffered silently
logger.Info(ctx, "Step 2 processing")   // Buffered silently
logger.Error(ctx, "Step 3 failed!")     // Triggers flush: prints Step 1, Step 2, and Step 3
```

## Deduplication Handler

The `Deduplication` handler suppresses duplicate log records that occur within a configurable time window (default 60 seconds). Records with level >= `dedupLevel` (default `ERROR`) are deduplicated, while records below `dedupLevel` pass through unconditionally. This protects logging and notification sinks from flood exhaustion during outages or retry storms.

```go
import (
	"context"
	"os"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

streamHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

// Deduplicate identical ERROR+ logs within a 60-second window
dedupHandler := handler.NewDeduplication(streamHandler, monogo.ERROR, 60*time.Second)

logger := monogo.New("app", []monogo.Handler{dedupHandler}, nil)

logger.Error(ctx, "Database connection lost") // Emitted immediately
logger.Error(ctx, "Database connection lost") // Suppressed (duplicate within 60s)
logger.Info(ctx, "User clicked button")       // Emitted (below dedupLevel)
```

## Handler Bubbling

Like PHP Monolog, handlers in Monogo are evaluated through a LIFO stack. By default, records bubble through all handlers that handle the record's level. A handler can stop propagation down the stack by configuring bubbling as `false` at construction time via `handler.WithBubble(false)`:

```go
ctx := context.Background()

// Error-only handler that absorbs ERROR logs and prevents them from reaching stdout (bubble = false)
errFile, _ := os.OpenFile("errors.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
errHandler := handler.NewStream(errFile, monogo.ERROR, handler.WithBubble(false))

stdoutHandler := handler.NewStream(os.Stdout, monogo.DEBUG) // default bubble = true

// Handlers are evaluated in stack order (errHandler runs first)
logger := monogo.New("app", []monogo.Handler{errHandler, stdoutHandler}, nil)

logger.Info(ctx, "Normal message")   // errHandler ignores; prints to stdout
logger.Error(ctx, "Critical error")  // errHandler handles and suppresses bubbling; only written to errors.log
```

## Per-Handler Processors

In addition to logger-level processors, Monogo supports **Per-Handler Processors** configured at construction time via `handler.WithProcessor(...)`:

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

ctx := context.Background()

// Add audit-specific metadata only to the audit log handler
auditFile, _ := os.OpenFile("audit.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
auditHandler := handler.NewStream(
	auditFile,
	monogo.INFO,
	handler.WithProcessor(processor.Tag("destination", "audit_trail")),
)

// Console handler receives records without the audit tag
consoleHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

logger := monogo.New("app", []monogo.Handler{auditHandler, consoleHandler}, nil)
logger.Info(ctx, "User logged in", map[string]interface{}{"user_id": 42})
```

### Handler Isolation
When per-handler processors are configured, the record is automatically cloned prior to executing the handler's processor pipeline. Any mutations made by a handler's processor (e.g. adding metadata, redacting sensitive fields, or modifying extra context) remain strictly isolated to that handler and will never leak to subsequent handlers down the logger stack.

## Batch Processing & Buffering

Buffering handlers accumulate log entries and flush them via `monogo.BatchHandler` and `monogo.BatchFormatter`. Handlers that support optimized batch emission receive batches directly via `HandleBatch`, while standard handlers gracefully receive records via `Handle` without requiring iteration boilerplate:

### 1. `Buffer` Handler
Buffers entries until a capacity limit is reached or a flush level is triggered:

```go
// Buffer up to 100 entries, flushing immediately if an ERROR occurs
fileHandler := handler.NewStream(file, monogo.DEBUG)
bufferHandler := handler.NewBuffer(fileHandler, 100, monogo.ERROR)

logger := monogo.New("app", []monogo.Handler{bufferHandler}, nil)
defer logger.Close(ctx) // Flushes remaining buffered logs on shutdown
```

### 2. `FingersCrossed` Handler
Buffers low-severity logs (e.g. `DEBUG`, `INFO`) and only flushes them if an action level (e.g. `ERROR`) is reached:

```go
// Retain up to 1000 records; silently buffers until an ERROR occurs, then flushes all diagnostic history
fileHandler := handler.NewStream(file, monogo.DEBUG)
fcHandler := handler.NewFingersCrossed(fileHandler, monogo.ERROR, 1000)

logger := monogo.New("app", []monogo.Handler{fcHandler}, nil)
```

### 3. Batch JSON Formatting Modes
The JSON formatter supports two batch formatting modes via `WithBatchMode`:

```go
// NDJSON format (one JSON object per line, default)
jsonLines := formatter.NewJSON("").WithBatchMode(formatter.BatchModeNewlines)

// JSON Array format (single JSON array containing all records in the batch)
jsonArray := formatter.NewJSON("").WithBatchMode(formatter.BatchModeJSON)
```


## Logging JSON to a File

Setting up Monogo to log formatted JSON records to a file is straightforward:

```go
package main

import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func main() {
	ctx := context.Background()

	// Open log file (create or append)
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create a Stream handler writing JSON logs to the file
	fileHandler := handler.NewStream(file, monogo.DEBUG, handler.WithFormatter(formatter.NewJSON("")))

	// Initialize Logger
	logger := monogo.New("app", []monogo.Handler{fileHandler}, nil)

	// Log JSON entries
	logger.Info(ctx, "Server started", map[string]interface{}{"port": 8080})
	logger.Error(ctx, "Database query failed", map[string]interface{}{"error": "timeout", "query_ms": 120})
}
```

## Using log/slog Backend

### 1. Send Monogo Logs to log/slog

```go
import (
	"context"
	"log/slog"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
)

ctx := context.Background()
slogHandler := slog.NewJSONHandler(os.Stdout, nil)
monoHandler := slogadapter.NewSlogHandler(slogHandler, monogo.DEBUG)

logger := monogo.New("app", []monogo.Handler{monoHandler}, nil)
logger.Info(ctx, "Logged via slog backend", map[string]interface{}{"env": "production"})
```

### 2. Route Standard log/slog calls to Monogo

```go
import (
	"log/slog"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
	"github.com/githoober/monogo/handler"
)

testHandler := handler.NewStream(os.Stdout, monogo.DEBUG)
monoLogger := monogo.New("bridge", []monogo.Handler{testHandler}, nil)

// Set standard slog default logger to use Monogo
slog.SetDefault(slog.New(slogadapter.NewMonogoSlogBridge(monoLogger)))

slog.Info("Hello from stdlib slog!", "key", "value")
```

## Using zerolog Backend

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/zerologadapter"
	"github.com/rs/zerolog"
)

ctx := context.Background()
zLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()
zh := zerologadapter.NewZerologHandler(zLogger, monogo.DEBUG)

logger := monogo.New("api", []monogo.Handler{zh}, nil)
logger.Error(ctx, "Database connection lost", map[string]interface{}{"db": "postgres"})
```

## Testing

Run all unit tests across all packages:

```bash
go test -v ./...
```
