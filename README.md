# Monogo

A flexible, channel-based generic structured logging library for Go inspired by PHP's Monolog. The core library is completely generic and decoupled from specific logging frameworks, allowing pluggable backend adapters like Go standard library `log/slog` and third-party loggers like `zerolog`.

## Features

- **Generic & Backend-Agnostic**: Core Monogo logger operates through generic `Handler`, `Processor`, and `Formatter` interfaces without hard dependencies on any specific backend.
- **Ambient Context Values**: Attach contextual fields (e.g., request ID, tenant ID, trace ID) to Go's `context.Context` using `monogo.WithContext` / `monogo.WithField`. These fields are automatically extracted and merged into log records on all log methods.
- **RFC 5424 / Monolog Log Levels**: `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY`.
- **Channel Support**: Easily categorize logs by channels (e.g. `app`, `auth`, `database`).
- **Handlers**: Stream, RotatingFile, Deduplication, FingersCrossed, Buffer, Filter, Group, WhatFailureGroup, Test, Null.
- **Per-Handler Processors**: Dedicated processor pipelines on individual handlers (`handler.WithProcessor(...)`) with copy-on-write record isolation to prevent mutation leakage across handlers.
- **Handler Bubbling Control**: Stop record propagation down the handler stack via `handler.WithBubble(false)` and the `monogo.Bubbler` interface.
- **First-Class Batch Processing**: Native `HandleBatch` and `FormatBatch` contracts across handlers and formatters for atomic, single-write flushing from buffering handlers (`Buffer`, `FingersCrossed`).
- **Resettable Lifecycle**: Modeled after Monolog's `ResettableInterface`, `monogo.Resettable` (`Reset(ctx context.Context) error`) allows loggers, handlers, and processors to reset buffers, re-arm triggers, clear deduplication stores, and regenerate request UIDs between jobs in long-running services.
- **Processors**: Enriched logging metadata (Caller, Hostname, Process ID/PID, Git build info, Environment variables, Memory stats, Tags, Unique request ID/UID).
- **Formatters**: Line, JSON (with NDJSON and JSON Array batch modes), Logfmt (canonical key=value format).
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

## WhatFailureGroup Handler

Inspired by PHP Monolog's `WhatFailureGroupHandler`, the `WhatFailureGroup` handler wraps a slice of handlers and suppresses any errors or panics returned by individual sub-handlers during `Handle`, `HandleBatch`, or `Close`. This guarantees that failures in secondary or external logging sinks (e.g., Slack webhooks, remote log aggregators, or Elasticsearch) never interrupt critical file/console logging or fail caller operations.

An optional error callback can be attached via `handler.WithWhatFailureCallback` to inspect and monitor suppressed errors (e.g., for metrics or diagnostics) without bubbling them to the caller:

```go
import (
	"context"
	"os"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/handler"
)

ctx := context.Background()

primaryFile, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
primaryHandler := handler.NewStream(primaryFile, monogo.INFO)

// Wrap non-critical or remote handlers in WhatFailureGroup
resilientGroup := handler.NewWhatFailureGroup(
	[]monogo.Handler{
		slackWebhookHandler,
		elasticsearchHandler,
	},
	handler.WithWhatFailureCallback(func(err error, h monogo.Handler) {
		// Log or record metrics for external sink failures without failing the caller
		metrics.Increment("logger.secondary_sink_failure")
	}),
)

logger := monogo.New("app", []monogo.Handler{primaryHandler, resilientGroup}, nil)

// Even if external services time out, panic, or fail, primaryHandler receives the log safely
logger.Error(ctx, "Payment transaction failed")
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

## Built-in Processors

Processors enrich log records with contextual and system diagnostic metadata before formatting and dispatching. Monogo includes the following built-in processors:

### Core Monolog Ports
- **`processor.Caller(skipFrames)`**: Injects calling source file, line number, and function name into `Extra["caller"]` (Monolog `IntrospectionProcessor`).
- **`processor.Hostname()`**: Injects the OS hostname into `Extra["hostname"]` (Monolog `HostnameProcessor`).
- **`processor.ProcessId()`**: Injects the current OS process ID (`os.Getpid()`) into `Extra["pid"]` (Monolog `ProcessIdProcessor`).
- **`processor.Memory()`**: Injects runtime memory allocation statistics (`alloc_bytes`, `total_alloc_bytes`, `sys_bytes`) into `Extra["memory"]` (Monolog `MemoryProcessor` / `MemoryUsageProcessor`).
- **`processor.UID(length...)`**: Injects a unique identifier string into `Extra["uid"]` that remains constant across log records and regenerates a fresh UID when `Reset(ctx)` is invoked (Monolog `UidProcessor`, implements `monogo.Resettable`).
- **`processor.Git(configs...)`**: Automatically discovers and injects Git commit hash, branch, time, and dirty status into `Extra["git"]` (Monolog `GitProcessor`, via Go's `runtime/debug.ReadBuildInfo()` or environment variables).
- **`processor.Tag(key, value)`**: Injects fixed key-value tags into `Record.Extra` (Monolog `TagProcessor`).

### Monogo Extensions
- **`processor.Env(keys...)`**: Extracts specified environment variables into `Extra["env"]` (convenience extension for containerized/cloud deployments).
- **`processor.EnvMap(mapping)`**: Maps environment variables directly to custom top-level keys in `Record.Extra`.

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
	"github.com/githoober/monogo/handler"
	"github.com/githoober/monogo/processor"
)

func main() {
	ctx := context.Background()

	// Configure handlers and processors
	streamH := handler.NewStream(os.Stdout, monogo.DEBUG)
	fcH := handler.NewFingersCrossed(streamH, monogo.ERROR, 100)

	logger := monogo.New("worker", []monogo.Handler{fcH}, []monogo.Processor{
		processor.UID(), // Injects unique trace UID per cycle
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

## Documentation

- [**Lineage, Adapted Counterparts & Go Innovations**](docs/comparison.md): Exhaustive breakdown of handlers, formatters, and concepts modeled after PHP Monolog core, what is new in Monogo (Go idioms and cloud extensions), and comparative analysis against popular Go loggers.
- [**Architecture & Design Decisions**](docs/architecture.md): Deep dive into core design choices, pluggable backend adapters (`log/slog`, `zerolog`), ambient context propagation, handler bubbling, per-handler processors, batch handling, deduplication filtering, and failure-tolerant grouping.
- [**Deliberately Unimplemented Features**](docs/deliberate_omissions.md): Details features intentionally omitted from Monolog (e.g. PSR-3 placeholder interpolation, `*f` methods, runtime setters) and their Go architectural rationales.
- [**Glossary & Concepts**](docs/glossary.md): Terminology and concepts including Channels, Handlers, Processors, Formatters, Bubbling, and Batching.
- [**Continuous Integration & Quality Assurance**](docs/ci.md): Details the GitHub Actions CI pipeline, test coverage, deadcode reachability checks, and `golangci-lint` static analysis.

## Testing & Continuous Integration

Monogo enforces strict quality standards via automated GitHub Actions CI and local tooling:

### Unit Tests, Coverage & Race Detection
Run tests across all packages with coverage profiling and race detection:
```bash
go test -v -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Deadcode Analysis
Verify zero dead or unreachable code using Go's official reachability analyzer:
```bash
go run golang.org/x/tools/cmd/deadcode@v0.51.0 -test ./...
```

### Linting (`golangci-lint` v2.14)
Run static analysis configured in `.golangci.yml` (enforces `staticcheck`, `errcheck`, `govet`, `ineffassign`, `unused`):
```bash
golangci-lint run
```

### GitHub Actions CI
The CI workflow defined in `.github/workflows/ci.yml` runs on every push and pull request to `main`, validating:
- **Test:** `go mod verify`, `go vet`, and tests with coverage
- **Deadcode:** call graph reachability analysis with zero allowable dead code
- **GolangCI-Lint:** automated multi-linter verification via `golangci-lint-action`
