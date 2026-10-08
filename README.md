# Monogo

[![CI](https://github.com/githoober/monogo/actions/workflows/ci.yml/badge.svg)](https://github.com/githoober/monogo/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/githoober/monogo.svg)](https://pkg.go.dev/github.com/githoober/monogo)
[![Go Version](https://img.shields.io/badge/go-1.24%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A flexible, channel-based generic structured logging library for Go inspired by PHP's Monolog.

Monogo is organized into a lightweight **Core module** with **zero third-party dependencies**, and an optional **Extension module (`ext`)** for advanced handlers, processors, middleware, and standard library adapters.

## Architecture & Modules

- **Core Module (`github.com/githoober/monogo`)**:
  - **Zero Third-Party Dependencies**: Pure Go standard library implementation.
  - **Generic Logging Engine**: Backend-agnostic `Handler`, `Processor`, and `Formatter` interfaces.
  - **Ambient Context Values**: Attach contextual fields to Go's `context.Context` via `monogo.WithContext` / `monogo.WithField`.
  - **RFC 5424 Log Levels & Channels**: 8 standard severity levels (`DEBUG` through `EMERGENCY`) and first-class channel segregation.
  - **Core Handlers**: `Stream`, dedicated `JSONStream` (`NewJSONStream` / `NewJSON`), `FingersCrossed`, `Test`, and `Null`.
  - **Core Processors**: `ProcessId` (`Process`), `Web` (HTTP request metadata extraction), `Env` / `EnvMap` (environment variable extraction).
  - **Core Formatters**: `Line`, `JSON` (NDJSON & JSON Array batch modes), `Logfmt` (canonical key=value format).
  - **Batching & Bubbling**: First-class `BatchHandler`, `BatchFormatter`, `Bubbler`, and `Resettable` lifecycle contracts.

- **Extension Module (`github.com/githoober/monogo/ext`)**:
  - **Advanced Handlers (`ext/handler`)**: `RotatingFile` (rolling log file rotation via `lumberjack.v2`), `Buffer`, `Deduplication`, `Sampling`, `Socket` (TCP/UDP/Unix), `Filter`, `Group`, `WhatFailureGroup`.
  - **Enriched Processors (`ext/processor`)**: `Caller` (introspection), `Hostname`, `Memory` (runtime stats), `UID` (request IDs), `Git` (build metadata), `Tag`.
  - **HTTP Middleware (`ext/middleware`)**: Standard `net/http` middleware with `X-Request-ID` generation, latency tracking, and request logging.
  - **Standard Library Adapters (`ext/adapter`)**:
    - `ext/adapter/slogadapter`: Bidirectional `log/slog` backend and bridge.
    - `ext/adapter/stdlogadapter`: `*log.Logger` and `io.Writer` bridge.

## Installation

Install the Core module (zero third-party dependencies):

```bash
go get github.com/githoober/monogo
```

Install the Extension module for advanced handlers, processors, adapters, and middleware:

```bash
go get github.com/githoober/monogo/ext
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
	// Create a stream handler writing to stdout (or handler.NewJSONStream for JSON)
	stdoutHandler := handler.NewStream(os.Stdout, monogo.INFO)

	// Create logger with Core ProcessId processor
	logger := monogo.New("main", []monogo.Handler{stdoutHandler}, []monogo.Processor{
		processor.ProcessId(),
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

## Dedicated JSONStream Handler (Core)

Monogo provides a dedicated `JSONStream` handler in the Core `handler` package (`handler.NewJSONStream` or `handler.NewJSON`). It preconfigures stream output with JSON formatting with zero boilerplate:

```go
package main

import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

func main() {
	ctx := context.Background()

	// Direct JSON logging to stdout or any io.Writer
	jsonHandler := handler.NewJSONStream(os.Stdout, monogo.DEBUG)
	logger := monogo.New("api", []monogo.Handler{jsonHandler}, nil)

	logger.Info(ctx, "Order processed", map[string]interface{}{"order_id": 1001, "amount": 49.99})
}
```

## Rotating JSON File & Log File Rotation (Core: Zero Dependencies)

For automatic log file rotation based on file size, backup retention count, max age, and optional compression, Monogo Core provides pure standard library implementations (`github.com/githoober/monogo/handler`) with **zero third-party dependencies**:
- **`handler.RotatingJSONFile`** (`handler.NewRotatingJSONFile` / `handler.NewJSONRotatingFile`): Dedicated rotating file handler pre-configured for structured JSON output.
- **`handler.RotatingFile`** (`handler.NewRotatingFile`): Standard rotating file handler configurable with any formatter (Line, JSON, Logfmt).
- **`handler.RotatingFileWriter`** (`handler.NewRotatingFileWriter`): Underlying thread-safe `io.WriteCloser` providing size rotation, backup count retention, age purging, and gzip compression (`compress/gzip`).

```go
package main

import (
	"context"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

func main() {
	ctx := context.Background()

	// Pure stdlib JSON log file with rotation (10MB limit, 5 backups, gzip compression, 30 days retention)
	rotJSONHandler := handler.NewRotatingJSONFile("logs/app.log", monogo.DEBUG,
		handler.WithMaxSizeMB(10),
		handler.WithMaxBackups(5),
		handler.WithMaxAgeDays(30),
		handler.WithCompress(true),
	)
	defer rotJSONHandler.Close(ctx)

	logger := monogo.New("app", []monogo.Handler{rotJSONHandler}, nil)
	logger.Info(ctx, "Order processed", map[string]interface{}{"order_id": 42, "amount": 99.5})
}
```

### Lumberjack-Backed Rotation (`ext/handler`)

For applications preferring `gopkg.in/natefinch/lumberjack.v2`, the `ext` module also provides an alternate `RotatingFile` handler:

```go
package main

import (
	"context"

	"github.com/githoober/monogo"
	exthandler "github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/formatter"
)

func main() {
	ctx := context.Background()

	rotHandler := exthandler.NewRotatingFile("app.log", monogo.DEBUG,
		exthandler.WithMaxSize(10),
		exthandler.WithMaxBackups(5),
		exthandler.WithMaxAge(30),
		exthandler.WithCompress(true),
		exthandler.WithFormatter(formatter.NewJSON("")),
	)
	defer rotHandler.Close(ctx)

	logger := monogo.New("app", []monogo.Handler{rotHandler}, nil)
	logger.Info(ctx, "App initialized with lumberjack rolling log files")
}
```

## FingersCrossed Handler (Core)

The `FingersCrossed` handler is included directly in the Core `handler` package (`github.com/githoober/monogo/handler`) with zero third-party dependencies. It buffers all low-level logs (such as DEBUG or INFO) silently and only flushes the entire buffer to a nested handler when a log record meets a specific action level (such as ERROR). Once triggered, it stays activated for subsequent logs.

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

## Deduplication Handler (`ext/handler`)

The `Deduplication` handler suppresses duplicate log records that occur within a configurable time window (default 60 seconds). Records with level >= `dedupLevel` (default `ERROR`) are deduplicated, while records below `dedupLevel` pass through unconditionally. This protects logging and notification sinks from flood exhaustion during outages or retry storms.

```go
import (
	"context"
	"os"
	"time"

	"github.com/githoober/monogo"
	exthandler "github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

streamHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

// Deduplicate identical ERROR+ logs within a 60-second window
dedupHandler := exthandler.NewDeduplication(streamHandler, monogo.ERROR, 60*time.Second)

logger := monogo.New("app", []monogo.Handler{dedupHandler}, nil)

logger.Error(ctx, "Database connection lost") // Emitted immediately
logger.Error(ctx, "Database connection lost") // Suppressed (duplicate within 60s)
logger.Info(ctx, "User clicked button")       // Emitted (below dedupLevel)
```

## WhatFailureGroup Handler (`ext/handler`)

Inspired by PHP Monolog's `WhatFailureGroupHandler`, the `WhatFailureGroup` handler wraps a slice of handlers and suppresses any errors or panics returned by individual sub-handlers during `Handle`, `HandleBatch`, or `Close`. This guarantees that failures in secondary or external logging sinks (e.g., Slack webhooks, remote log aggregators, or Elasticsearch) never interrupt critical file/console logging or fail caller operations.

An optional error callback can be attached via `exthandler.WithWhatFailureCallback` to inspect and monitor suppressed errors (e.g., for metrics or diagnostics) without bubbling them to the caller:

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	exthandler "github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

primaryFile, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
primaryHandler := handler.NewStream(primaryFile, monogo.INFO)

// Wrap non-critical or remote handlers in WhatFailureGroup
resilientGroup := exthandler.NewWhatFailureGroup(
	[]monogo.Handler{
		slackWebhookHandler,
		elasticsearchHandler,
	},
	exthandler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
		// Log or record metrics for external sink failures without failing the caller
		metrics.Increment("logger.secondary_sink_failure")
	}),
)

logger := monogo.New("app", []monogo.Handler{primaryHandler, resilientGroup}, nil)

// Even if external services time out, panic, or fail, primaryHandler receives the log safely
logger.Error(ctx, "Payment transaction failed")
```

## Sampling Handler (`ext/handler`)

Inspired by PHP Monolog's `SamplingHandler`, the `Sampling` handler downsamples high-throughput log traffic based on a 1-in-N sampling factor (e.g., factor `10` emits approximately 10% of records). To prevent losing critical operational errors, `exthandler.WithSamplingThreshold` allows logs at or above a specified severity level (e.g. `ERROR`) to completely bypass sampling:

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	exthandler "github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

stdoutHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

// Sample DEBUG and INFO logs 1-in-10 (10%), but always emit ERROR+ logs (100%)
samplingHandler := exthandler.NewSampling(
	stdoutHandler,
	10,
	exthandler.WithSamplingThreshold(monogo.ERROR),
)

logger := monogo.New("app", []monogo.Handler{samplingHandler}, nil)

logger.Debug(ctx, "High volume trace")  // Emitted with 10% probability
logger.Error(ctx, "Critical failure")    // Always emitted (bypasses sampling)
```

## Socket Handler (`ext/handler`)

Modeled after PHP Monolog's `SocketHandler`, the `Socket` handler writes formatted log records over network sockets (TCP, UDP, or Unix domain sockets). It features automatic reconnection, customizable dial/write timeouts, and `Resettable` lifecycle support:

```go
import (
	"context"
	"time"

	"github.com/githoober/monogo"
	exthandler "github.com/githoober/monogo/ext/handler"
	"github.com/githoober/monogo/formatter"
)

ctx := context.Background()

// Stream logs over TCP to Logstash / remote syslog / aggregator
socketHandler := exthandler.NewSocket(
	"tcp",
	"10.0.0.50:5000",
	monogo.INFO,
	exthandler.WithFormatter(formatter.NewJSON("")),
	exthandler.WithWriteTimeout(3*time.Second),
	exthandler.WithDialTimeout(5*time.Second),
)
defer socketHandler.Close(ctx)

logger := monogo.New("network-app", []monogo.Handler{socketHandler}, nil)
logger.Info(ctx, "Log streaming over TCP socket")
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
	extprocessor "github.com/githoober/monogo/ext/processor"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

// Add audit-specific metadata only to the audit log handler
auditFile, _ := os.OpenFile("audit.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
auditHandler := handler.NewStream(
	auditFile,
	monogo.INFO,
	handler.WithProcessor(extprocessor.Tag("destination", "audit_trail")),
)

// Console handler receives records without the audit tag
consoleHandler := handler.NewStream(os.Stdout, monogo.DEBUG)

logger := monogo.New("app", []monogo.Handler{auditHandler, consoleHandler}, nil)
logger.Info(ctx, "User logged in", map[string]interface{}{"user_id": 42})
```

### Handler Isolation
When per-handler processors are configured, the record is automatically cloned prior to executing the handler's processor pipeline. Any mutations made by a handler's processor (e.g. adding metadata, redacting sensitive fields, or modifying extra context) remain strictly isolated to that handler and will never leak to subsequent handlers down the logger stack.

## Built-in Processors

Processors enrich log records with contextual and system diagnostic metadata before formatting and dispatching.

### Core Processors (`github.com/githoober/monogo/processor`)

Zero external dependencies, always available in core:
- **`processor.ProcessId()`** (or `processor.Process()`): Injects current OS process ID (`os.Getpid()`) into `Extra["pid"]` (Monolog `ProcessIdProcessor`).
- **`processor.Web(opts...)`**: Injects HTTP request metadata (URL, client IP, method, server, referrer, user agent) into `Extra` from context (Monolog `WebProcessor`). Use `processor.WithHTTPRequest(ctx, req)` to attach the HTTP request to the context.
- **`processor.Env(keys...)`**: Extracts specified environment variables into `Extra["env"]` (convenience extension for containerized/cloud deployments).
- **`processor.EnvMap(mapping)`**: Maps environment variables directly to custom top-level keys in `Record.Extra`.

### Extension Processors (`github.com/githoober/monogo/ext/processor`)

Extended diagnostic processors in the `ext` module:
- **`processor.Caller(skipFrames)`**: Injects calling source file, line number, and function name into `Extra["caller"]` (Monolog `IntrospectionProcessor`).
- **`processor.Hostname()`**: Injects the OS hostname into `Extra["hostname"]` (Monolog `HostnameProcessor`).
- **`processor.Memory()`**: Injects runtime memory allocation statistics (`alloc_bytes`, `total_alloc_bytes`, `sys_bytes`) into `Extra["memory"]` (Monolog `MemoryProcessor` / `MemoryUsageProcessor`).
- **`processor.UID(length...)`**: Injects a unique identifier string into `Extra["uid"]` that remains constant across log records and regenerates a fresh UID when `Reset(ctx)` is invoked (Monolog `UidProcessor`, implements `monogo.Resettable`).
- **`processor.Git(configs...)`**: Automatically discovers and injects Git commit hash, branch, time, and dirty status into `Extra["git"]` (Monolog `GitProcessor`, via Go's `runtime/debug.ReadBuildInfo()` or environment variables).
- **`processor.Tag(key, value)`**: Injects fixed key-value tags into `Record.Extra` (Monolog `TagProcessor`).

## Batch Processing & Buffering

Buffering handlers accumulate log entries and flush them via `monogo.BatchHandler` and `monogo.BatchFormatter`. Handlers that support optimized batch emission receive batches directly via `HandleBatch`, while standard handlers gracefully receive records via `Handle` without requiring iteration boilerplate:

### 1. `Buffer` Handler (`ext/handler`)
Buffers entries until a capacity limit is reached or a flush level is triggered:

```go
// Buffer up to 100 entries, flushing immediately if an ERROR occurs
fileHandler := handler.NewStream(file, monogo.DEBUG)
bufferHandler := exthandler.NewBuffer(fileHandler, 100, monogo.ERROR)

logger := monogo.New("app", []monogo.Handler{bufferHandler}, nil)
defer logger.Close(ctx) // Flushes remaining buffered logs on shutdown
```

### 2. `FingersCrossed` Handler (Core)
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

## Resettable Interface & Long-Running Services

Modeled after PHP Monolog's `ResettableInterface`, the `monogo.Resettable` interface (`Reset(ctx context.Context) error`) allows long-running Go applications (background workers, task consumers, HTTP servers, or test runners) to cleanly reset state between requests or jobs:

```go
type Resettable interface {
	Reset(ctx context.Context) error
}
```

### Cascading Reset & Concurrency Barrier
Calling `logger.Reset(ctx)` automatically traverses the logger stack while acting as a concurrency barrier (waiting for any in-flight `Log` calls to finish, and blocking new `Log` calls until reset finishes):
- **Handlers:** Dispatches `Reset(ctx)` to each configured handler implementing `Resettable`.
  - **`Buffer`**: Flushes buffered records using `ctx` to the wrapped handler, resets per-handler processors, and resets wrapped handlers. Returns any flush error. (Or use `handler.Clear()` to discard records without flushing).
  - **`FingersCrossed`**: Disarms triggered status back to buffering mode, clears the buffer, and resets inner handlers. (Or use `handler.Clear()` to discard records).
  - **`Deduplication`**: Clears the sliding deduplication store and resets wrapped handlers.
  - **`Group` & `WhatFailureGroup`**: Cascades `Reset(ctx)` across all child handlers (with `WhatFailureGroup` safely suppressing and reporting panics via callback).
  - **`Test`**: Clears recorded log records in memory.
- **Processors:** Dispatches `Reset(ctx)` to each configured processor implementing `Resettable`.
  - **`processor.UID()`**: Regenerates a fresh request/operation identifier in `Extra["uid"]`.

### Example: Worker Pool Cycle
```go
package main

import (
	"context"
	"os"

	"github.com/githoober/monogo"
	extprocessor "github.com/githoober/monogo/ext/processor"
	"github.com/githoober/monogo/handler"
)

func main() {
	ctx := context.Background()

	// Configure handlers and processors
	streamH := handler.NewStream(os.Stdout, monogo.DEBUG)
	fcH := handler.NewFingersCrossed(streamH, monogo.ERROR, 100)

	logger := monogo.New("worker", []monogo.Handler{fcH}, []monogo.Processor{
		extprocessor.UID(), // Injects unique trace UID per cycle
	})

	// Process jobs in a worker loop
	jobs := []string{"job-101", "job-102", "job-103"}
	for _, jobID := range jobs {
		// Reset logger between jobs: regenerates UID and re-arms FingersCrossed buffer
		_ = logger.Reset(ctx)

		logger.Info(ctx, "Starting job", map[string]interface{}{"job_id": jobID})
		// If error occurs, FingersCrossed triggers and dumps full diagnostic logs with job's UID
		if jobID == "job-102" {
			logger.Error(ctx, "Failed to process job", map[string]interface{}{"job_id": jobID})
		}
	}
}
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

## Logfmt Formatter (Go Cloud Extension)

As a built-in extension beyond PHP Monolog tailored for the Go cloud ecosystem, Monogo provides the `Logfmt` formatter (`formatter.Logfmt`, aliased as `formatter.LogfmtFormatter`), which formats structured log records into canonical `key=value` logfmt lines (standard for Grafana Loki, Promtail, Heroku, and Go CLI conventions):

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

	// Create a stream handler using Logfmt formatting
	logfmtHandler := handler.NewStream(
		os.Stdout,
		monogo.DEBUG,
		handler.WithFormatter(formatter.NewLogfmt(
			formatter.WithTimeKey("ts"),
			formatter.WithLevelKey("lvl"),
			formatter.WithChannelKey("channel"),
			formatter.WithMessageKey("msg"),
		)),
	)

	logger := monogo.New("app", []monogo.Handler{logfmtHandler}, nil)
	logger.Info(ctx, "User logged in", map[string]interface{}{"user_id": 42, "ip": "192.168.1.1"})
	// Output:
	// ts=2026-10-04T12:00:00Z lvl=INFO channel=app msg="User logged in" ip=192.168.1.1 user_id=42
}
```

## Using log/slog Backend (`ext/adapter/slogadapter`)

### 1. Send Monogo Logs to log/slog

```go
import (
	"context"
	"log/slog"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/adapter/slogadapter"
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
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/adapter/slogadapter"
	"github.com/githoober/monogo/handler"
)

testHandler := handler.NewStream(os.Stdout, monogo.DEBUG)
monoLogger := monogo.New("bridge", []monogo.Handler{testHandler}, nil)

// Set standard slog default logger to use Monogo
slog.SetDefault(slog.New(slogadapter.NewMonogoSlogBridge(monoLogger)))

slog.Info("Hello from stdlib slog!", "key", "value")
```

## Using Standard Library Bridge (*log.Logger & io.Writer - `ext/adapter/stdlogadapter`)

To integrate Monogo with standard library servers (such as `http.Server.ErrorLog`), database drivers, or legacy Go packages that write to an `io.Writer` or standard library `*log.Logger`, the `ext/adapter/stdlogadapter` package routes incoming log lines into a `*monogo.Logger` at a designated level:

```go
import (
	"context"
	"net/http"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/adapter/stdlogadapter"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()
logger := monogo.New("server", []monogo.Handler{handler.NewStream(os.Stdout, monogo.INFO)}, nil)

// 1. Pass standard library *log.Logger to http.Server
server := &http.Server{
	Addr:     ":8080",
	ErrorLog: stdlogadapter.NewStdLogger(ctx, logger, monogo.ERROR, "[http] ", 0),
}

// 2. Or obtain an io.Writer for third-party libraries
writer := stdlogadapter.NewWriter(ctx, logger, monogo.INFO)
```

## HTTP Middleware (`ext/middleware`) & WebProcessor (Core)

Monogo includes an idiomatic Go `net/http` middleware (`ext/middleware.HTTP`) and a Monolog-compatible `processor.Web` in Core:
- **`middleware.HTTP`**: Intercepts requests, automatically assigns/propagates `X-Request-ID`, extracts request metadata into ambient context via `processor.WithHTTPRequest`, records response status codes, latency, and bytes written, and logs request completions.
- **`processor.Web`**: Injects request attributes (`url`, `ip`, `http_method`, `server`, `referrer`, `user_agent`) into `Record.Extra` from request context.

```go
package main

import (
	"net/http"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/ext/middleware"
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func main() {
	// Logger configured with WebProcessor to enrich log records with HTTP metadata
	logger := monogo.New("api",
		[]monogo.Handler{handler.NewStream(os.Stdout, monogo.DEBUG)},
		[]monogo.Processor{processor.Web()},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		// Log from inside handler; ambient request_id and HTTP attributes are automatically attached!
		logger.Info(r.Context(), "Greeting user", map[string]interface{}{"user": "alice"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Wrap entire router with Monogo HTTP middleware
	httpHandler := middleware.HTTP(logger)(mux)
	_ = http.ListenAndServe(":8080", httpHandler)
}
```

## Documentation

- [**Lineage, Adapted Counterparts & Go Innovations**](docs/comparison.md): Exhaustive breakdown of handlers, formatters, and concepts modeled after PHP Monolog core, what is new in Monogo (Go idioms and cloud extensions), and comparative analysis against popular Go loggers.
- [**Architecture & Design Decisions**](docs/architecture.md): Deep dive into core design choices, pluggable backend adapters (`log/slog`), ambient context propagation, handler bubbling, per-handler processors, batch handling, deduplication filtering, and failure-tolerant grouping.
- [**Deliberately Unimplemented Features**](docs/deliberate_omissions.md): Details features intentionally omitted from Monolog (e.g. PSR-3 placeholder interpolation, `*f` methods, runtime setters) and their Go architectural rationales.
- [**Glossary & Concepts**](docs/glossary.md): Terminology and concepts including Channels, Handlers, Processors, Formatters, Bubbling, and Batching.
- [**Continuous Integration & Quality Assurance**](docs/ci.md): Details the GitHub Actions CI pipeline, test coverage, deadcode reachability checks, and `golangci-lint` static analysis.

## Testing & Continuous Integration

Monogo enforces strict quality standards across both the Core (`.`) and Extension (`./ext`) modules via automated GitHub Actions CI and local tooling:

### Unit Tests, Coverage & Race Detection
Run tests across both modules with coverage profiling and race detection:
```bash
go test -v -race -count=1 ./... ./ext/...
```

### Deadcode Analysis
Verify zero dead or unreachable code across both modules using Go's official reachability analyzer:
```bash
go run golang.org/x/tools/cmd/deadcode@v0.51.0 -test ./... ./ext/...
```

### Linting (`golangci-lint` v2.14)
Run static analysis configured in `.golangci.yml` across both modules:
```bash
golangci-lint run ./...
(cd ext && golangci-lint run ./...)
```

### GitHub Actions CI
The CI workflow defined in `.github/workflows/ci.yml` runs on every push and pull request to `main`, validating:
- **Test:** `go mod verify`, `go vet`, and tests with race detection across `./...` and `./ext/...`
- **Deadcode:** reachability analysis with zero allowable dead code across `./...` and `./ext/...`
- **GolangCI-Lint:** automated multi-linter verification for both modules via `golangci-lint-action`

