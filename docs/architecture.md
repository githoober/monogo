# Architectural Decisions: Monogo

This document outlines the primary architectural decisions behind **Monogo** (`github.com/githoober/monogo`), a Go port of PHP Monolog.

## 1. Modular Multi-Module Architecture (Model 1 Split)

### Decision
The repository is split into two decoupled Go modules:
1. **Core Module (`github.com/githoober/monogo`)**:
   - Strictly backend-agnostic with **100% zero third-party dependencies** (relies entirely on Go standard library).
   - Defines core interfaces (`Handler`, `BatchHandler`, `Bubbler`, `ProcessableHandler`, `Resettable`, `Processor`, `ProcessorFunc`, `Formatter`, `BatchFormatter`) and data structures (`Record`, `Level`).
   - Contains core handlers (`Stream`, dedicated `JSONStream` via `handler.NewJSONStream` / `handler.NewJSON`, `FingersCrossed`, `Test`, `Null`).
   - Contains core formatters (`Line`, `JSON`, `Logfmt`).
   - Contains core processors (`ProcessId` / `Process`, `Web` HTTP metadata extractor).
2. **Extension Module (`github.com/githoober/monogo/ext`)**:
   - Advanced handlers (`RotatingFile` powered by `lumberjack.v2`, `Buffer`, `Deduplication`, `Sampling`, `Socket`, `Filter`, `Group`, `WhatFailureGroup`).
   - Extended diagnostic processors (`Caller`, `Hostname`, `Memory`, `UID`, `Git`, `Tag`, `Env`, `EnvMap`).
   - HTTP middleware (`net/http` request logging, latency tracking, `X-Request-ID`).
   - Pluggable standard library adapters (`ext/adapter/slogadapter`, `ext/adapter/stdlogadapter`).

### Rationale
In PHP Monolog, handlers dictate how log records are handled and emitted. In Go, applications value minimal dependency trees and fast build times. Consumers needing only a lightweight, zero-dependency logging core and pipeline can depend exclusively on `github.com/githoober/monogo`. Applications needing rolling files (`lumberjack`), network sockets, complex deduplication caches, or framework adapters can opt into `github.com/githoober/monogo/ext` without forcing third-party dependencies onto core consumers.

---

## 2. Pluggable Standard Library Adapters (`ext/adapter/`)

### Decision
Standard library integrations reside in separate subpackages under `ext/adapter/`:
- **`ext/adapter/slogadapter`**: Provides bidirectional `log/slog` integration (`SlogHandler` sends Monogo records to any `slog.Handler`; `MonogoSlogBridge` implements `slog.Handler` using Monogo as the backend).
- **`ext/adapter/stdlogadapter`**: Provides `NewWriter` and `NewStdLogger` to route standard library `*log.Logger` and `io.Writer` outputs directly into Monogo.

### Rationale
By focusing on native standard library interfaces (`log/slog` and `*log.Logger`), Monogo enables universal interoperability across the entire Go ecosystem (including third-party loggers like `zerolog` or `zap`, which provide native `slog.Handler` implementations) without polluting the core module.

---

## 3. Ambient Context Values via `context.Context`

### Decision
Ambient contextual fields (such as `request_id`, `trace_id`, or `tenant_id`) can be stored in Go's standard `context.Context` using `monogo.WithContext(ctx, fields)` or `monogo.WithField(ctx, key, value)`.

All log methods strictly require `context.Context` as their first parameter (e.g. `logger.Info(ctx, ...)`). Ambient fields are automatically extracted via `monogo.FromContext(ctx)` and merged into `Record.Context`. Furthermore, `ctx context.Context` is passed explicitly through all handler contracts (`Handler.IsHandling(ctx, level)`, `Handler.Handle(ctx, record)`, `BatchHandler.HandleBatch(ctx, records)`, and `Handler.Close(ctx)`), allowing context-aware sinks (like `slogadapter.SlogHandler`) to receive the client context directly.

### Rationale
In Go, `context.Context` is the standard mechanism for passing request-scoped values across API boundaries and goroutines. Passing `context.Context` explicitly as the first argument prevents accidental context-dropping, avoids storing contexts inside data structs (`Record`), and ensures log entries consistently carry ambient metadata.

---

## 4. Child Loggers & Channels

### Decision
Monogo supports two types of child logger creation:
1. **Contextual Child Loggers (`logger.With(map[string]interface{})`)**: Creates a new `*Logger` instance that prepends a custom processor to automatically attach fixed fields to every record.
2. **Channel Child Loggers (`logger.WithName(name)` / `logger.WithChannel(name)`)**: Creates a new `*Logger` instance that inherits the parent's handler stack and processors while overriding the channel name.

### Rationale
This matches Monolog's channel-centric logging model (e.g., separating `app`, `auth`, `database` logs) while providing a familiar API for attaching component-level metadata.

---

## 5. RFC 5424 Log Level System

### Decision
Monogo implements RFC 5424 log levels (`DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY`) as an integer-backed `Level` type with integer steps of 100.

### Rationale
This matches PHP Monolog's level hierarchy, allowing fine-grained log filtering while offering conversion helpers to map to Go `log/slog` levels (`DEBUG`, `INFO`, `WARN`, `ERROR`).

---

## 6. Pipeline & Handlers

### Decision
Log records flow through the classic Monolog pipeline (IsHandling -> Processors -> Handlers -> Formatters).

**Core Handlers (`github.com/githoober/monogo/handler`)**:
- **`Stream`**: Writes formatted logs to any `io.Writer`.
- **`JSONStream`**: Dedicated stream handler preconfigured with JSON formatting (`NewJSONStream` / `NewJSON`).
- **`FingersCrossed`**: Buffers low-level logs until an action level (e.g., `ERROR`) triggers flushing all buffered logs.
- **`Test`**: In-memory record retention for unit test verification.
- **`Null`**: Consumes and discards records silently.

**Extension Handlers (`github.com/githoober/monogo/ext/handler`)**:
- **`RotatingFile`**: Leverages `lumberjack.v2` for size/age/compression-based rolling log file rotation.
- **`Deduplication`**: Suppresses identical log records that occur within a configurable time window (e.g. 60s) to prevent log flooding during outages.
- **`WhatFailureGroup`**: Wraps a group of handlers and swallows any errors or panics returned by individual handlers during `Handle`, `HandleBatch`, or `Close`, preventing secondary sink failures from breaking primary logging.
- **`Filter`**, **`Group`**, **`Buffer`**, **`Sampling`**, **`Socket`**.

Handlers support propagation control (bubbling) configured at construction time via options (`handler.WithBubble(...)`) and the `monogo.Bubbler` interface. If a handler processes a record and its `Bubble()` returns `false`, record propagation halts, preventing subsequent handlers down the stack from receiving it.

Handlers also support per-handler processors via `handler.WithProcessor(...)` and implement `monogo.ProcessableHandler`. Handler-specific processors execute just before formatting and dispatching. When processors are configured on a handler, the record is automatically cloned first, strictly isolating modifications (such as adding destination-specific extra tags or credentials masking) from subsequent handlers in the stack.

Handlers and formatters also implement batch operations (`HandleBatch`, `FormatBatch`), allowing buffering handlers (`Buffer`, `FingersCrossed`) to flush accumulated records in atomic bulk operations without per-record locking overhead.

For flood control, `Deduplication` acts as a decorator handler. Unlike PHP Monolog which relies on disk files to share deduplication state between ephemeral PHP processes, Monogo utilizes a high-performance, thread-safe in-memory cache with zero-goroutine periodic auto-pruning to eliminate memory leaks in 24/7 long-running services, while supporting custom `DeduplicationStore` backends (e.g. distributed caches).

For error isolation across multiple handlers, `WhatFailureGroup` guarantees that unreliable sinks (remote syslog, webhooks, Slack, Elasticsearch) can fail or panic without disrupting healthy handlers or crashing application requests.

### Rationale
Providing high-utility Monolog handlers allows developers to easily construct production-grade logging setups with rolling files, buffering, error-triggered flushes, duplicate suppression, or resilient failure swallowing. Bubbling control allows dedicated handlers (such as alert/error handlers) to absorb specific logs without cluttering general output handlers. Per-handler processors allow customizing records for specific destinations without polluting other log targets. First-class batching ensures buffering handlers flush efficiently and atomically. Deduplication protects logging, alerting, and notification sinks from flood exhaustion during cascading failures, retry loops, or outages. WhatFailureGroup ensures secondary and external logging sinks do not create single points of failure in applications.

---

## 7. Two-Tier Processors & Copy-On-Write Handler Isolation

### Decision
Monogo implements a two-tier processor architecture:
1. **Logger-Level Processors (`monogo.New(..., processors)` / `logger.PushProcessor(p)`)**: Execute on every record at the logger boundary before any handler is invoked. Ideal for global enrichment (e.g. hostname, process ID, environment tags).
2. **Per-Handler Processors (`handler.WithProcessor(p...)`)**: Configured at construction time on individual handlers. Handlers implement `monogo.ProcessableHandler` via `handler.BaseHandler`.

Available processors:
- **Core (`github.com/githoober/monogo/processor`)**: `ProcessId` (`Process`), `Web` (HTTP metadata extraction).
- **Extension (`github.com/githoober/monogo/ext/processor`)**: `Caller`, `Hostname`, `Memory`, `UID`, `Git`, `Tag`, `Env`, `EnvMap`.

When a handler executes its processor pipeline via `ProcessRecord`:
- If no processors are configured, the record is returned immediately with zero allocations.
- If processors are configured, `record = record.Clone()` makes deep copies of `Context` and `Extra` maps before running the handler's processors.

### Rationale
In PHP Monolog, handlers mutate the `LogRecord` object directly. In concurrent Go environments, mutable shared state across multiple handlers causes data races and unwanted mutation leakage (e.g., an audit handler adding destination-specific tags that leak into a general file log). Copy-on-write cloning in `ProcessRecord` guarantees complete pipeline isolation: destination-specific transformations (such as PII redaction, credential masking, or destination labeling) apply exclusively to that specific handler. Furthermore, using construction-time functional options (`handler.WithProcessor`) rather than mutable runtime setters preserves thread safety and immutability during log dispatch.
