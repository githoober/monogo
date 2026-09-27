# Monogo

A flexible, channel-based generic structured logging library for Go inspired by PHP's Monolog. The core library is completely generic and decoupled from specific logging frameworks, allowing pluggable backend adapters like Go standard library `log/slog` and third-party loggers like `zerolog`.

## Features

- **Generic & Backend-Agnostic**: Core Monogo logger operates through generic `Handler`, `Processor`, and `Formatter` interfaces without hard dependencies on any specific backend.
- **Ambient Context Values**: Attach contextual fields (e.g., request ID, tenant ID, trace ID) to Go's `context.Context` using `monolog.WithContext` / `monolog.WithField`. These fields are automatically extracted and merged into log records when using `*Context` log methods.
- **RFC 5424 / Monolog Log Levels**: `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY`.
- **Channel Support**: Easily categorize logs by channels (e.g. `app`, `auth`, `database`).
- **Handlers**: Stream, RotatingFile, Filter, Group, Buffer, FingersCrossed, Test, Null.
- **Processors**: Enriched logging metadata (Caller, Hostname, Memory stats, Tags, Unique request ID/UID).
- **Formatters**: Line, JSON.
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
	stdoutHandler := handler.NewStream(os.Stdout, monolog.INFO)

	// Create logger
	logger := monolog.New("main", []monolog.Handler{stdoutHandler}, []monolog.Processor{
		processor.Hostname(),
		processor.UID(),
	})

	// Set ambient fields in Go context
	ctx := context.Background()
	ctx = monolog.WithField(ctx, "request_id", "req-12345")
	ctx = monolog.WithContext(ctx, map[string]interface{}{
		"tenant": "acme-corp",
	})

	// Log messages with context; ambient context values are automatically included
	logger.InfoContext(ctx, "User logged in", map[string]interface{}{"user_id": 42})
	logger.WarningContext(ctx, "Rate limit approaching", map[string]interface{}{"ip": "127.0.0.1"})
}
```

## Log File Rotation (RotatingFile)

Monogo provides a RotatingFile handler powered by lumberjack for automatic log file rotation based on file size, backup retention count, age, and optional compression:

```go
package main

import (
	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func main() {
	// Create a rotating file handler (rotates when log reaches 10MB, keeps 5 backups, retains for 30 days, compresses, formatted as JSON)
	rotHandler := handler.NewRotatingFile("app.log", monolog.DEBUG,
		handler.WithMaxSize(10),
		handler.WithMaxBackups(5),
		handler.WithMaxAge(30),
		handler.WithCompress(true),
		handler.WithFormatter(formatter.NewJSON("")),
	)
	defer rotHandler.Close()

	logger := monolog.New("app", []monolog.Handler{rotHandler}, nil)
	logger.Info("App initialized with rolling log files")
}
```

## FingersCrossed Handler

The FingersCrossed handler buffers all low-level logs (such as DEBUG or INFO) silently and only flushes the entire buffer to a nested handler when a log record meets a specific action level (such as ERROR). Once triggered, it stays activated for subsequent logs.

```go
import (
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

// Create a stream output target
streamHandler := handler.NewStream(os.Stdout, monolog.DEBUG)

// Wrap with FingersCrossed: buffers until ERROR level is triggered (buffer size up to 100 entries)
fcHandler := handler.NewFingersCrossed(streamHandler, monolog.ERROR, 100)

logger := monolog.New("app", []monolog.Handler{fcHandler}, nil)

logger.Debug("Step 1 initialized") // Buffered silently
logger.Info("Step 2 processing")   // Buffered silently
logger.Error("Step 3 failed!")     // Triggers flush: prints Step 1, Step 2, and Step 3
```

## Handler Bubbling

Like PHP Monolog, handlers in Monogo are evaluated through a LIFO stack. By default, records bubble through all handlers that handle the record's level. A handler can stop propagation down the stack by configuring bubbling as `false` at construction time via `handler.WithBubble(false)`:

```go
// Error-only handler that absorbs ERROR logs and prevents them from reaching stdout (bubble = false)
errFile, _ := os.OpenFile("errors.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
errHandler := handler.NewStream(errFile, monolog.ERROR, handler.WithBubble(false))

stdoutHandler := handler.NewStream(os.Stdout, monolog.DEBUG) // default bubble = true

// Handlers are evaluated in stack order (errHandler runs first)
logger := monolog.New("app", []monolog.Handler{errHandler, stdoutHandler}, nil)

logger.Info("Normal message")   // errHandler ignores; prints to stdout
logger.Error("Critical error")  // errHandler handles and suppresses bubbling; only written to errors.log
```


## Logging JSON to a File

Setting up Monogo to log formatted JSON records to a file is straightforward:

```go
package main

import (
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func main() {
	// Open log file (create or append)
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create a Stream handler writing JSON logs to the file
	fileHandler := handler.NewStream(file, monolog.DEBUG, handler.WithFormatter(formatter.NewJSON("")))

	// Initialize Logger
	logger := monolog.New("app", []monolog.Handler{fileHandler}, nil)

	// Log JSON entries
	logger.Info("Server started", map[string]interface{}{"port": 8080})
	logger.Error("Database query failed", map[string]interface{}{"error": "timeout", "query_ms": 120})
}
```

## Using log/slog Backend

### 1. Send Monogo Logs to log/slog

```go
import (
	"log/slog"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
)

slogHandler := slog.NewJSONHandler(os.Stdout, nil)
monoHandler := slogadapter.NewSlogHandler(slogHandler, monolog.DEBUG)

logger := monolog.New("app", []monolog.Handler{monoHandler}, nil)
logger.Info("Logged via slog backend", map[string]interface{}{"env": "production"})
```

### 2. Route Standard log/slog calls to Monogo

```go
import (
	"log/slog"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/slogadapter"
	"github.com/githoober/monogo/handler"
)

testHandler := handler.NewStream(os.Stdout, monolog.DEBUG)
monoLogger := monolog.New("bridge", []monolog.Handler{testHandler}, nil)

// Set standard slog default logger to use Monogo
slog.SetDefault(slog.New(slogadapter.NewMonologSlogBridge(monoLogger)))

slog.Info("Hello from stdlib slog!", "key", "value")
```

## Using zerolog Backend

```go
import (
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/adapter/zerologadapter"
	"github.com/rs/zerolog"
)

zLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()
zh := zerologadapter.NewZerologHandler(zLogger, monolog.DEBUG)

logger := monolog.New("api", []monolog.Handler{zh}, nil)
logger.Error("Database connection lost", map[string]interface{}{"db": "postgres"})
```

## Testing

Run all unit tests across all packages:

```bash
go test -v ./...
```
