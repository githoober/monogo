# Monogo vs. PHP Monolog: Lineage, Adapted Counterparts & Go Innovations

This document details the provenance and design of **Monogo** (`github.com/githoober/monogo`), providing an explicit, transparent accounting of:
1. **Handlers, formatters, and concepts modeled after PHP Monolog core** (`Seldaek/monolog`).
2. **Fundamental language and paradigm differences** between PHP and Go.
3. **What is new in Monogo** (idiomatic Go innovations, cloud-native adaptations, and extensions).
4. **What is deliberately omitted** from Monolog (and why).
5. **How Monogo compares against other popular Go logging frameworks** (`log/slog`, `logrus`, `zap`, `zerolog`).

---

## 1. Components Modeled After PHP Monolog Core

Monogo preserves the core architecture, data model, and processing pipeline of PHP Monolog (`Seldaek/monolog`), implementing adapted Go counterparts for its primary components:

### Core Architecture & Concepts
| Feature / Concept | PHP Monolog Implementation | Monogo Implementation | Details |
| :--- | :--- | :--- | :--- |
| **Severity Levels** | `Logger::DEBUG` (100) .. `EMERGENCY` (600) | `monogo.DEBUG` (100) .. `EMERGENCY` (600) | 8 RFC 5424 levels with integer steps of 100. |
| **Channels** | First-class channel string per logger | First-class channel string (`Record.Channel`, `logger.WithChannel()`) | Segregates logs by application subsystem (`app`, `auth`, `db`). |
| **Record Data Model** | `Monolog\LogRecord` | `monogo.Record` | Strictly separates call-site event data (`Context`) from processor-injected metadata (`Extra`). |
| **Handler Pipeline** | LIFO stack evaluation | Slice evaluation (`[]monogo.Handler`) | Log records flow sequentially through configured handlers. |
| **Bubbling Control** | `$bubble = false` on `AbstractProcessingHandler` | `monogo.Bubbler` interface & `handler.WithBubble(bool)` | Prevents record propagation down the handler stack when a handler consumes the record. |
| **Per-Handler Processors** | `ProcessableHandlerInterface` / `pushProcessor` | `monogo.ProcessableHandler` & `handler.WithProcessor(...)` | Allows individual handlers to attach dedicated processors. |
| **Batch Processing** | `HandlerInterface::handleBatch` & `FormatterInterface::formatBatch` | `monogo.BatchHandler` & `monogo.BatchFormatter` | Modeled after Monolog's batch contracts. In PHP Monolog, `handleBatch` is mandatory on `HandlerInterface` and `formatBatch` on `FormatterInterface` (relying on base class `foreach` loops). Monogo adapts this using Go's Interface Segregation Principle (`BatchHandler` / `BatchFormatter` are optional interfaces checked via type assertion, falling back automatically to single-record `Handle` loops). |
| **Resettable State** | `Monolog\ResettableInterface` (`reset(): void`) | `monogo.Resettable` (`Reset(ctx) error`) | Resets internal buffers, deduplication stores, and processor states (such as `processor.UID`) between log cycles or worker jobs in long-running processes. Modeled after Monolog's `ResettableInterface`; `logger.Reset(ctx)` acts as a concurrency barrier and cascades down through all handlers and processors with context propagation and error return. |

---

### Language & Paradigm Translations
| Paradigm Aspect | PHP Monolog | Monogo | Idiomatic Go Rationale |
| :--- | :--- | :--- | :--- |
| **Type System & OOP** | Classes, Interfaces, Inheritance | Structs, Small Interfaces, Embedding | Go favors composition over deep inheritance hierarchies. Shared handler logic is embedded via `handler.BaseHandler`. |
| **Error Handling** | Exceptions (`throw \Exception`) | Explicit `error` return values | Idiomatic Go error handling throughout pipeline contracts (`Handle`, `Format`, `Close`). |
| **Call-site Fields** | Array `['user' => 42]` | Variadic `...map[string]interface{}` & `context.Context` | Supports both in-place map literals and Go's ambient `context.Context` propagation. |
| **Processors** | Callable `function(LogRecord $r)` | `Processor` interface & `ProcessorFunc` | Mirrors Go's standard `http.Handler` / `http.HandlerFunc` functional pattern. |
| **Concurrency** | Single-threaded PHP request lifecycle | Goroutine-safe (`sync.RWMutex` / `sync.Mutex`) | Safely usable across thousands of concurrent goroutines in 24/7 web servers and background workers. |

---

### Handlers Modeled After Core Monolog (`Monolog\Handler\*`)
The handlers below are adapted counterparts modeled after upstream PHP Monolog core classes, tailored to Go's runtime and ecosystem:

| Monogo Handler | Package & Location | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- | :--- |
| [`handler.Stream`](../handler/stream.go) | `handler` (Core) | `Monolog\Handler\StreamHandler` | Writes formatted records to any `io.Writer` (console `os.Stdout`/`os.Stderr`, files, network sockets). |
| [`handler.JSONStream`](../handler/json.go) | `handler` (Core) | `Monolog\Handler\StreamHandler` | Dedicated stream handler preconfigured with JSON formatting (`NewJSONStream` / `NewJSON`). |
| [`handler.RotatingFile`](../handler/rotating_file.go) | `handler` (Core) | `Monolog\Handler\RotatingFileHandler` | Pure standard library size/daily log file rotation with backup retention, max age cleanup, and optional gzip compression. Zero external dependencies. |
| [`handler.RotatingJSONFile`](../handler/rotating_file.go) | `handler` (Core) | `Monolog\Handler\RotatingFileHandler` | Dedicated rotating file handler preconfigured with JSON formatting (`NewRotatingJSONFile` / `NewJSONRotatingFile`). Pure standard library. |
| [`handler.FingersCrossed`](../handler/fingers_crossed.go) | `handler` (Core) | `Monolog\Handler\FingersCrossedHandler` | Buffers low-severity diagnostic records (`DEBUG`, `INFO`) silently until an action level (e.g. `ERROR`) is encountered, then flushes full history. |
| [`handler.Null`](../handler/test_null.go) | `handler` (Core) | `Monolog\Handler\NullHandler` | Consumes and discards all log records without action (useful for muting logs in tests or specific channels). |
| [`handler.Test`](../handler/test_null.go) | `handler` (Core) | `Monolog\Handler\TestHandler` | Retains records in memory for assertions during unit and integration testing. |
| [`handler.RotatingFile`](../ext/handler/rotating_file.go) | `ext/handler` | `Monolog\Handler\RotatingFileHandler` | Alternate counterpart to `RotatingFileHandler` powered by `lumberjack.v2`. Provided in `ext/handler` for lumberjack users. |
| [`handler.Buffer`](../ext/handler/buffer.go) | `ext/handler` | `Monolog\Handler\BufferHandler` | Buffers records and flushes on capacity limit, action level, or `Close`. |
| [`handler.Filter`](../ext/handler/filter.go) | `ext/handler` | `Monolog\Handler\FilterHandler` | Passes records only if their level falls within an inclusive min/max level range; drops out-of-range records. |
| [`handler.Group`](../ext/handler/group.go) | `ext/handler` | `Monolog\Handler\GroupHandler` | Multiplexes log records to a slice of nested child handlers. |
| [`handler.Deduplication`](../ext/handler/deduplication.go) | `ext/handler` | `Monolog\Handler\DeduplicationHandler` | Provides sliding time-window duplicate suppression using a thread-safe in-memory cache with auto-pruning. |
| [`handler.WhatFailureGroup`](../ext/handler/what_failure_group.go) | `ext/handler` | `Monolog\Handler\WhatFailureGroupHandler` | Multiplexes records to child handlers while safely swallowing and suppressing all errors and recovered panics. |
| [`handler.Sampling`](../ext/handler/sampling.go) | `ext/handler` | `Monolog\Handler\SamplingHandler` | Downsamples records based on a 1-in-N sampling factor, supporting custom sampler strategies and level thresholds. |
| [`handler.Socket`](../ext/handler/socket.go) | `ext/handler` | `Monolog\Handler\SocketHandler` | Streams formatted log records over network sockets (TCP, UDP, Unix domain sockets) with automatic reconnection and timeouts. |

---

### Formatters Modeled After Core Monolog (`Monolog\Formatter\*`)

| Monogo Formatter | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- |
| [`formatter.Line`](../formatter/line.go) | `Monolog\Formatter\LineFormatter` | Formats records into customizable text lines with timestamp, channel, level, message, and serialized context/extra. |
| [`formatter.JSON`](../formatter/json.go) | `Monolog\Formatter\JsonFormatter` | Formats records into structured JSON payloads suitable for log shippers and ingestion systems. |

---

### Processors Modeled After Core Monolog (`Monolog\Processor\*`)

| Monogo Processor | Package & Location | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- | :--- |
| [`processor.ProcessId`](../processor/process.go) | `processor` (Core) | `Monolog\Processor\ProcessIdProcessor` | Injects the current operating system process ID (`os.Getpid()`) into `Extra["pid"]`. Aliased as `processor.Process()`. |
| [`processor.Web`](../processor/web.go) | `processor` (Core) | `Monolog\Processor\WebProcessor` | Injects HTTP request attributes (URL, client IP, method, server, referrer, user agent) into `Record.Extra` from request context. |
| [`processor.Caller`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\IntrospectionProcessor` | Extracts source file, line number, and function name of the log call site using Go's `runtime.Caller`. |
| [`processor.Hostname`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\HostnameProcessor` | Injects the machine hostname into `Extra["hostname"]` via `os.Hostname()`. |
| [`processor.Memory`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\MemoryUsageProcessor` / `MemoryPeakUsageProcessor` | Injects Go runtime memory statistics (`alloc_bytes`, `total_alloc_bytes`, `sys_bytes` from `runtime.MemStats`). |
| [`processor.UID`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\UidProcessor` | Injects a unique identifier string into `Extra["uid"]`; regenerates a new UID when `Reset(ctx)` is invoked. |
| [`processor.Git`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\GitProcessor` | Injects Git commit hash, branch, time, and dirty status into `Extra["git"]` via Go build info (`runtime/debug.ReadBuildInfo`) and environment variables. |
| [`processor.Tag`](../ext/processor/processor.go) | `ext/processor` | `Monolog\Processor\TagProcessor` | Injects arbitrary fixed key-value tags into `Record.Extra`. |

---

## 2. What Is New in Monogo (Go-Specific Innovations & Extensions)

While Monogo mirrors Monolog's architecture, Go's runtime characteristics (goroutines, static typing, explicit error handling, cloud containerization) require distinct design patterns:

### 1. `Logfmt` Formatter
- **Status:** **New in Monogo** *(Not in PHP Monolog core)*.
- **What it does:** [`formatter.Logfmt`](../formatter/logfmt.go) (aliased as `formatter.LogfmtFormatter`) formats log records into canonical `key=value` logfmt lines (e.g. `ts=2026-10-04T12:00:00Z lvl=INFO channel=app msg="User logged in"`). Implements `monogo.Formatter` and `monogo.BatchFormatter`.
- **Rationale:** Logfmt is a ubiquitous, lightweight format in the Go cloud-native ecosystem (popularized by Grafana Loki, Promtail, Heroku, and Go CLI tools). In PHP Monolog, logfmt was never part of core and only existed as third-party community packages. Monogo provides built-in first-class logfmt support with configurable keys, prefixes, rune-aware key sanitization, and JSON-compatible control character escaping.

### 2. `Env` & `EnvMap` Processors
- **Status:** **New in Monogo** *(Not in PHP Monolog core)*.
- **What it does:** [`processor.Env(keys...)`](../processor/env.go) extracts specified environment variables into `Extra["env"]`, while [`processor.EnvMap(mapping)`](../processor/env.go) maps environment variables directly to top-level keys in `Record.Extra`.
- **Rationale:** In containerized cloud environments (Kubernetes, AWS ECS, GCP Cloud Run), runtime metadata such as `POD_NAME`, `NAMESPACE`, `CLUSTER`, or `DEPLOY_ENV` is injected via environment variables. Providing built-in environment processors enables zero-boilerplate injection of container metadata.

### 3. First-Class `context.Context` Architecture
- **Status:** **New in Monogo** *(Go standard library idiom)*.
- **What it does:**
  - `ctx context.Context` is strictly the mandatory first argument on all `Logger` level methods (`logger.Info(ctx, ...)`), handler operations (`Handler.Handle(ctx, record)`, `BatchHandler.HandleBatch(ctx, records)`), and lifecycle methods (`Handler.Close(ctx)`).
  - Ambient contextual fields can be attached to Go contexts via `monogo.WithContext(ctx, fields)` or `monogo.WithField(ctx, key, value)` and are automatically extracted and merged into log records across goroutines.
  - Unlike common anti-patterns, `context.Context` is **not stored inside the `Record` struct**. `Record` remains a clean, serializable data carrier, while `ctx` travels explicitly through function parameters.

### 4. Zero External Dependencies in Core & Modular Extension Module (`ext`)
- **Status:** **New in Monogo** *(Dependency isolation)*.
- **What it does:**
  - The root `monogo` package relies exclusively on the Go standard library (zero external dependencies).
  - Extended handlers and ecosystem integrations live in the `github.com/githoober/monogo/ext` module:
    - `ext/adapter/slogadapter.NewSlogHandler`: Routes Monogo log records to any standard library `slog.Handler`.
    - `ext/adapter/slogadapter.NewMonogoSlogBridge`: Implements `slog.Handler`, allowing standard library `log/slog` calls to be routed through the Monogo processing pipeline.
    - `ext/adapter/stdlogadapter`: Provides `NewWriter` (`io.Writer`) and `NewStdLogger` (`*log.Logger`), enabling standard library HTTP servers and third-party tools to pipe logs into Monogo.
    - `ext/handler`: Extended handlers (`RotatingFile` via `lumberjack.v2`, `Buffer`, `Deduplication`, `Sampling`, `Socket`, etc.).
    - `ext/processor`: Extended diagnostic processors (`Caller`, `Hostname`, `Memory`, `UID`, `Git`, `Tag`).
    - `ext/middleware`: HTTP server middleware.
  - Consumers importing core Monogo pull in zero unwanted third-party dependencies.

### 5. Concurrent Goroutine Safety & Copy-On-Write Isolation
- **Status:** **New in Monogo** *(Concurrency paradigm difference)*.
- **What it does:**
  - PHP Monolog executes in single-threaded request lifecycles where loggers are instantiated per request. In Go, a single `*monogo.Logger` and its handlers are shared concurrently across thousands of goroutines.
  - All Monogo components are strictly thread-safe using `sync.RWMutex` / `sync.Mutex`.
  - **Copy-On-Write Handler Isolation:** In PHP Monolog, handlers mutate the `LogRecord` directly. In Monogo, when per-handler processors are configured on a handler, `record.Clone()` makes deep copies of `Context` and `Extra` maps before running the handler's processors. Destination-specific mutations (such as masking secrets or adding destination tags) cannot leak to subsequent handlers in the stack or race across goroutines.

### 6. Construction-Time Immutability via Functional Options
- **Status:** **New in Monogo** *(Idiomatic Go configuration)*.
- **What it does:**
  - Replaces PHP Monolog's mutable runtime setters (`$handler->setLevel(...)`, `$handler->setFormatter(...)`) with immutable functional options (`handler.WithBubble`, `handler.WithFormatter`, `handler.WithProcessor`).
  - Guarantees handlers and loggers are immutable after creation, eliminating data races on read/write paths during high-throughput logging.

### 7. In-Memory Auto-Pruning for Long-Running Processes
- **Status:** **New in Monogo** *(Process lifecycle difference)*.
- **What it does:**
  - PHP Monolog's `DeduplicationHandler` wrote state to local disk files because PHP processes die at the end of each HTTP request.
  - Monogo's `Deduplication` handler uses a high-performance in-memory cache with zero-goroutine lazy auto-pruning. It avoids disk I/O, prevents memory leaks in 24/7 services, and supports pluggable custom store backends (`DeduplicationStore`).

### 8. Segregated Optional Batch Interfaces
- **Status:** **New in Monogo** *(Interface Segregation Principle)*.
- **What it does:**
  - Instead of forcing `HandleBatch` onto every single handler struct (which would require dummy loop boilerplate for custom handlers), Monogo segregates `Handler` from `BatchHandler` and `Formatter` from `BatchFormatter`.
  - Buffering handlers (`Buffer`, `FingersCrossed`) detect `BatchHandler` via runtime type assertion (`if bh, ok := h.(BatchHandler); ok`), executing atomic bulk writes when supported and automatically falling back to single-record `Handle` loops when not.

### 9. Batch JSON Formatting Modes
- **Status:** **New in Monogo**.
- **What it does:**
  - `formatter.JSON` supports two batch formatting modes via `WithBatchMode`:
    - `BatchModeNewlines` (default): Formats batches as newline-delimited JSON (NDJSON).
    - `BatchModeJSON`: Formats the entire batch as a single JSON array (`[...]`).

### 10. Native `net/http` Middleware
- **Status:** **New in Monogo** *(Go web ecosystem standard)*.
- **What it does:**
  - `ext/middleware.HTTP(logger)` (`../ext/middleware/http.go`) provides a standard `func(http.Handler) http.Handler` middleware that assigns/preserves `X-Request-ID`, binds ambient request metadata via `processor.WithHTTPRequest`, records response status codes and bytes written, measures duration, and logs completed requests.

---

## 3. Deliberately Omitted from Monolog

Several PHP Monolog and PSR-3 features were intentionally omitted from Monogo due to Go architectural and performance considerations. See [Deliberately Unimplemented Features](deliberate_omissions.md) for full rationales:

| Omitted Feature | PHP Monolog / PSR-3 | Rationale in Monogo |
| :--- | :--- | :--- |
| **Message Placeholder Interpolation** | `{username}` replaced from context | Modern observability treats log messages as low-cardinality static templates. Variable attributes belong in structured context maps. Eliminates string scanning overhead on hot paths. |
| **Formatted `*f` Methods** | `Logf`, `Infof`, `Errorf` | Encourages baking variables into unstructured strings instead of structured attributes. Callers can use `fmt.Sprintf` explicitly when needed. |
| **Duplicate `*Context` Methods** | `Info` vs `InfoContext` | `ctx context.Context` is mandatory on all level methods. Eliminates API duplication and prevents accidental context dropping. |
| **Storing Context in Record** | Storing `ctx` inside struct | Violates the official Go Context rule: *"Do not store Contexts inside a struct type"*. Context is passed explicitly as function parameters. |
| **Runtime Mutators / Setters** | `setLevel`, `setFormatter` | Dynamic runtime mutation causes data races in multi-threaded Go. Configuration is immutable via functional options. |
| **Recursive Object Reflection** | `NormalizerFormatter` | Deep reflection traversal is slow and can infinite-loop on cyclic data. Go utilizes explicit interfaces (`json.Marshaler`, `fmt.Stringer`). |
| **Synchronous Network Sinks** | `NativeMailerHandler`, `SlackWebhookHandler` | In-process synchronous network calls create latency spikes and single points of failure. In 12-factor cloud apps, logs stream to stdout/local forwarders (Vector, Fluent Bit, OTel). |

---

## 4. Comparison with Popular Go Loggers

| Feature / Concept | Monogo | `log/slog` (Stdlib) | `sirupsen/logrus` | `uber-go/zap` | `rs/zerolog` |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Channel Support** | First-class (`Record.Channel`) | Not built-in | Not built-in | Logger name | Logger component |
| **Call-site Fields** | `Context` map & variadic maps | `Attr` / variadic args | `WithFields` | Strongly-typed `Field`s | Chained `Fields` |
| **System Metadata** | `Extra` map (via Processors) | Handler wrappers | Hooks | Core / Encoders | Event hooks |
| **Handler Pipeline** | Pipeline stack (`Deduplication`, `FingersCrossed`, `Buffer`, `Stream`, `WhatFailureGroup`) | Single `slog.Handler` | Hooks / `io.Writer` | `zapcore.Core` | `io.Writer` |
| **Two-Tier Processors** | Logger-level & Per-Handler processors with copy-on-write isolation | Not built-in | Hooks (global only) | Not built-in | Hooks |
| **Propagation Control** | Bubbling control (`handler.WithBubble(false)`) | Not built-in | Not built-in | Not built-in | Not built-in |
| **Formatters** | Segregated `Formatter` & `BatchFormatter` (`Line`, `JSON`, `Logfmt`) | `TextHandler` / `JSONHandler` | `Formatter` (`Text`, `JSON`) | Encoders | Console / JSON |
| **Interoperability** | Bidirectional `slog` bridge + stdlib `*log.Logger` / `io.Writer` bridge | Native | Via wrappers | Via `zapio` | Native |
