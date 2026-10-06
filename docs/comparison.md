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

| Monogo Handler | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- |
| [`handler.Stream`](../handler/stream.go) | `Monolog\Handler\StreamHandler` | Writes formatted records to any `io.Writer` (console `os.Stdout`/`os.Stderr`, files, network sockets). |
| [`handler.RotatingFile`](../handler/rotating_file.go) | `Monolog\Handler\RotatingFileHandler` | Adapted counterpart to `RotatingFileHandler`. Whereas Monolog rotates on calendar dates (one file per day), Monogo adapts rotation for long-running Go services by rotating on file size, max age, and backup retention using `lumberjack.v2`. |
| [`handler.Buffer`](../handler/buffer.go) | `Monolog\Handler\BufferHandler` | Adapted counterpart to `BufferHandler`. Buffers records and flushes on capacity limit, action level, or `Close` (adapting Monolog's request-lifecycle buffering for long-running Go services). |
| [`handler.FingersCrossed`](../handler/fingers_crossed.go) | `Monolog\Handler\FingersCrossedHandler` | Adapted counterpart to `FingersCrossedHandler`. Buffers low-severity diagnostic records (`DEBUG`, `INFO`) silently until an action level (e.g. `ERROR`) is encountered, then flushes full history. |
| [`handler.Filter`](../handler/filter.go) | `Monolog\Handler\FilterHandler` | Adapted counterpart to `FilterHandler`. Passes records only if their level falls within an inclusive min/max level range; drops out-of-range records. |
| [`handler.Group`](../handler/group.go) | `Monolog\Handler\GroupHandler` | Adapted counterpart to `GroupHandler`. Multiplexes log records to a slice of nested child handlers. |
| [`handler.Null`](../handler/test_null.go) | `Monolog\Handler\NullHandler` | Adapted counterpart to `NullHandler`. Consumes and discards all log records without action (useful for muting logs in tests or specific channels). |
| [`handler.Test`](../handler/test_null.go) | `Monolog\Handler\TestHandler` | Adapted counterpart to `TestHandler`. Retains records in memory for assertions during unit and integration testing. |
| [`handler.Deduplication`](../handler/deduplication.go) | `Monolog\Handler\DeduplicationHandler` | Adapted counterpart to `DeduplicationHandler`. Provides sliding time-window duplicate suppression, adapted to use a thread-safe in-memory cache with auto-pruning rather than Monolog's file-based store. |
| [`handler.WhatFailureGroup`](../handler/what_failure_group.go) | `Monolog\Handler\WhatFailureGroupHandler` | Adapted counterpart to `WhatFailureGroupHandler`. Multiplexes records to child handlers while safely swallowing and suppressing all errors and recovered panics (analogous to catching `Throwable` in PHP). |
| [`handler.Sampling`](../handler/sampling.go) | `Monolog\Handler\SamplingHandler` | Adapted counterpart to `SamplingHandler`. Downsamples records based on a 1-in-N sampling factor, supporting custom sampler strategies and level thresholds to bypass sampling for critical logs. |

---

### Formatters Modeled After Core Monolog (`Monolog\Formatter\*`)

| Monogo Formatter | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- |
| [`formatter.Line`](../formatter/line.go) | `Monolog\Formatter\LineFormatter` | Formats records into customizable text lines with timestamp, channel, level, message, and serialized context/extra. |
| [`formatter.JSON`](../formatter/json.go) | `Monolog\Formatter\JsonFormatter` | Formats records into structured JSON payloads suitable for log shippers and ingestion systems. |

---

### Processors Modeled After Core Monolog (`Monolog\Processor\*`)

| Monogo Processor | PHP Monolog Class | Purpose & Adaptation Details |
| :--- | :--- | :--- |
| [`processor.Caller`](../processor/processor.go) | `Monolog\Processor\IntrospectionProcessor` | Extracts source file, line number, and function name of the log call site using Go's `runtime.Caller` instead of PHP's `debug_backtrace()`. |
| [`processor.Hostname`](../processor/processor.go) | `Monolog\Processor\HostnameProcessor` | Injects the machine hostname into `Extra["hostname"]` via `os.Hostname()`. |
| [`processor.Memory`](../processor/processor.go) | `Monolog\Processor\MemoryUsageProcessor` / `MemoryPeakUsageProcessor` | Injects Go runtime memory statistics (`alloc_bytes`, `total_alloc_bytes`, `sys_bytes` from `runtime.MemStats`) instead of PHP's `memory_get_usage()`. |
| [`processor.UID`](../processor/processor.go) | `Monolog\Processor\UidProcessor` | Injects a unique identifier string into `Extra["uid"]` to trace operations across a lifecycle; regenerates a new UID when `Reset(ctx)` is invoked (implements `monogo.Resettable`). |
| [`processor.ProcessId`](../processor/processor.go) | `Monolog\Processor\ProcessIdProcessor` | Injects the current operating system process ID (`os.Getpid()`) into `Extra["pid"]`. |
| [`processor.Git`](../processor/processor.go) | `Monolog\Processor\GitProcessor` | Injects Git commit hash, branch, time, and dirty status into `Extra["git"]` via Go build info (`runtime/debug.ReadBuildInfo`) and environment variables instead of git CLI execution. |
| [`processor.Tag`](../processor/processor.go) | `Monolog\Processor\TagProcessor` | Injects arbitrary fixed key-value tags into `Record.Extra`. |

---

## 2. What Is New in Monogo (Go-Specific Innovations & Extensions)

While Monogo mirrors Monolog's architecture, Go's runtime characteristics (goroutines, static typing, explicit error handling, cloud containerization) require distinct design patterns:

### 1. `Logfmt` Formatter
- **Status:** **New in Monogo** *(Not in PHP Monolog core)*.
- **What it does:** [`formatter.Logfmt`](../formatter/logfmt.go) (aliased as `formatter.LogfmtFormatter`) formats log records into canonical `key=value` logfmt lines (e.g. `ts=2026-10-04T12:00:00Z lvl=INFO channel=app msg="User logged in"`). Implements `monogo.Formatter` and `monogo.BatchFormatter`.
- **Rationale:** Logfmt is a ubiquitous, lightweight format in the Go cloud-native ecosystem (popularized by Grafana Loki, Promtail, Heroku, and Go CLI tools). In PHP Monolog, logfmt was never part of core and only existed as third-party community packages. Monogo provides built-in first-class logfmt support with configurable keys, prefixes, rune-aware key sanitization, and JSON-compatible control character escaping.

### 2. `Env` & `EnvMap` Processors
- **Status:** **New in Monogo** *(Not in PHP Monolog core)*.
- **What it does:** [`processor.Env(keys...)`](../processor/processor.go) extracts specified environment variables into `Extra["env"]`, while [`processor.EnvMap(mapping)`](../processor/processor.go) maps environment variables directly to top-level keys in `Record.Extra`.
- **Rationale:** In containerized cloud environments (Kubernetes, AWS ECS, GCP Cloud Run), runtime metadata such as `POD_NAME`, `NAMESPACE`, `CLUSTER`, or `DEPLOY_ENV` is injected via environment variables. Providing built-in environment processors enables zero-boilerplate injection of container metadata.

### 3. First-Class `context.Context` Architecture
- **Status:** **New in Monogo** *(Go standard library idiom)*.
- **What it does:**
  - `ctx context.Context` is strictly the mandatory first argument on all `Logger` level methods (`logger.Info(ctx, ...)`), handler operations (`Handler.Handle(ctx, record)`, `BatchHandler.HandleBatch(ctx, records)`), and lifecycle methods (`Handler.Close(ctx)`).
  - Ambient contextual fields can be attached to Go contexts via `monogo.WithContext(ctx, fields)` or `monogo.WithField(ctx, key, value)` and are automatically extracted and merged into log records across goroutines.
  - Unlike common anti-patterns, `context.Context` is **not stored inside the `Record` struct**. `Record` remains a clean, serializable data carrier, while `ctx` travels explicitly through function parameters.

### 4. Zero External Dependencies in Core & Pluggable Backend Adapters
- **Status:** **New in Monogo** *(Dependency isolation)*.
- **What it does:**
  - The root `monogo` package relies exclusively on the Go standard library.
  - Framework-specific integrations live in isolated subpackages:
    - `adapter/slogadapter.NewSlogHandler`: Routes Monogo log records to any standard library `slog.Handler`.
    - `adapter/slogadapter.NewMonogoSlogBridge`: Implements `slog.Handler`, allowing standard library `log/slog` calls to be routed through the Monogo processing pipeline.
    - `adapter/zerologadapter.New`: Routes Monogo log records to `rs/zerolog`.
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
| **Interoperability** | Bidirectional `slog` bridge + `zerolog` adapter | Native | Via wrappers | Via `zapio` | Native |
