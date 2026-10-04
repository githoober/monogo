# Monogo vs. PHP Monolog: Lineage, Parity & Go Innovations

This document details the provenance of features in **Monogo** (`github.com/githoober/monogo`), providing an explicit, transparent accounting of:
1. **What came directly from PHP Monolog core** (`Seldaek/monolog`).
2. **What is new in Monogo** (idiomatic Go innovations, cloud-native adaptations, and extensions).
3. **What is deliberately omitted** from Monolog (and why).
4. **How Monogo compares against other popular Go logging frameworks** (`log/slog`, `logrus`, `zap`, `zerolog`).

---

## 1. What Came Directly from PHP Monolog (Upstream Parity)

Monogo preserves the core architecture, data model, and processing pipeline of PHP Monolog (`Seldaek/monolog`), adapting them to idiomatic Go:

### Core Architecture & Concepts
| Feature / Concept | PHP Monolog Implementation | Monogo Implementation | Details |
| :--- | :--- | :--- | :--- |
| **Severity Levels** | `Logger::DEBUG` (100) .. `EMERGENCY` (600) | `monogo.DEBUG` (100) .. `EMERGENCY` (600) | 8 RFC 5424 levels with integer steps of 100. |
| **Channels** | First-class channel string per logger | First-class channel string (`Record.Channel`, `logger.WithChannel()`) | Segregates logs by application subsystem (`app`, `auth`, `db`). |
| **Record Data Model** | `Monolog\LogRecord` | `monogo.Record` | Strictly separates call-site event data (`Context`) from processor-injected metadata (`Extra`). |
| **Handler Pipeline** | LIFO stack evaluation | Slice evaluation (`[]monogo.Handler`) | Log records flow sequentially through configured handlers. |
| **Bubbling Control** | `$bubble = false` on `AbstractProcessingHandler` | `monogo.Bubbler` interface & `handler.WithBubble(bool)` | Prevents record propagation down the handler stack when a handler consumes the record. |
| **Per-Handler Processors** | `ProcessableHandlerInterface` / `pushProcessor` | `monogo.ProcessableHandler` & `handler.WithProcessor(...)` | Allows individual handlers to attach dedicated processors. |
| **Batch Processing** | `HandlerInterface::handleBatch` & `FormatterInterface::formatBatch` | `monogo.BatchHandler` & `monogo.BatchFormatter` | Directly ported from Monolog's batch contracts. In PHP Monolog, `handleBatch` is mandatory on `HandlerInterface` and `formatBatch` on `FormatterInterface` (relying on base class `foreach` loops). Monogo adapts this using Go's Interface Segregation Principle (`BatchHandler` / `BatchFormatter` are optional interfaces checked via type assertion, falling back automatically to single-record `Handle` loops). |

---

### Handlers Ported from Core Monolog (`Monolog\Handler\*`)
Every handler below corresponds 1:1 to an upstream PHP Monolog core handler:

| Monogo Handler | PHP Monolog Class | Purpose & Parity Details |
| :--- | :--- | :--- |
| [`handler.Stream`](../handler/stream.go) | `Monolog\Handler\StreamHandler` | Writes formatted records to any `io.Writer` (console `os.Stdout`/`os.Stderr`, files, network sockets). |
| [`handler.RotatingFile`](../handler/rotating_file.go) | `Monolog\Handler\RotatingFileHandler` | Rotates log files based on file size, retention count, and age (powered by `lumberjack.v2`). |
| [`handler.Buffer`](../handler/buffer.go) | `Monolog\Handler\BufferHandler` | Buffers records in memory until a capacity limit is reached or a trigger level (e.g. `ERROR`) is reached, flushing all buffered records. |
| [`handler.FingersCrossed`](../handler/fingers_crossed.go) | `Monolog\Handler\FingersCrossedHandler` | Buffers low-severity diagnostic records (`DEBUG`, `INFO`) silently until an action level (e.g. `ERROR`) is encountered, then flushes full history. |
| [`handler.Filter`](../handler/filter.go) | `Monolog\Handler\FilterHandler` | Passes records only if their level falls within an inclusive min/max level range; drops out-of-range records. |
| [`handler.Group`](../handler/group.go) | `Monolog\Handler\GroupHandler` | Multiplexes log records to a slice of nested child handlers. |
| [`handler.Null`](../handler/test_null.go) | `Monolog\Handler\NullHandler` | Consumes and discards all log records without action (useful for muting logs in tests or specific channels). |
| [`handler.Test`](../handler/test_null.go) | `Monolog\Handler\TestHandler` | Retains records in memory for assertions during unit and integration testing. |
| [`handler.Deduplication`](../handler/deduplication.go) | `Monolog\Handler\DeduplicationHandler` | Suppresses duplicate records that recur within a sliding time window (default 60s) to protect alert sinks from flood exhaustion. |
| [`handler.WhatFailureGroup`](../handler/what_failure_group.go) | `Monolog\Handler\WhatFailureGroupHandler` | Multiplexes records to child handlers while safely swallowing and suppressing all errors and panics, ensuring non-critical sinks (webhooks, Elasticsearch) cannot fail the application. |

---

### Formatters Ported from Core Monolog (`Monolog\Formatter\*`)

| Monogo Formatter | PHP Monolog Class | Purpose & Parity Details |
| :--- | :--- | :--- |
| [`formatter.Line`](../formatter/line.go) | `Monolog\Formatter\LineFormatter` | Formats records into customizable text lines with timestamp, channel, level, message, and serialized context/extra. |
| [`formatter.JSON`](../formatter/json.go) | `Monolog\Formatter\JsonFormatter` | Formats records into structured JSON payloads suitable for log shippers and ingestion systems. |

---

### Processors Ported from Core Monolog (`Monolog\Processor\*`)

| Monogo Processor | PHP Monolog Class | Purpose & Parity Details |
| :--- | :--- | :--- |
| [`processor.Caller`](../processor/processor.go) | `Monolog\Processor\IntrospectionProcessor` | Extracts source file, line number, and function name of the log call site and adds to `Extra["caller"]`. |
| [`processor.Hostname`](../processor/processor.go) | `Monolog\Processor\HostnameProcessor` | Injects the machine hostname into `Extra["hostname"]`. |
| [`processor.Memory`](../processor/processor.go) | `Monolog\Processor\MemoryUsageProcessor` / `MemoryPeakUsageProcessor` | Injects Go runtime memory statistics (`alloc_bytes`, `total_alloc_bytes`, `sys_bytes`) into `Extra["memory"]`. |
| [`processor.UID`](../processor/processor.go) | `Monolog\Processor\UidProcessor` | Injects a random unique identifier string into `Extra["uid"]` to trace operations. |
| [`processor.ProcessId`](../processor/processor.go) | `Monolog\Processor\ProcessIdProcessor` | Injects the current operating system process ID (`os.Getpid()`) into `Extra["pid"]`. |
| [`processor.Git`](../processor/processor.go) | `Monolog\Processor\GitProcessor` | Injects Git commit hash, branch, time, and dirty status into `Extra["git"]` (via Go `runtime/debug.ReadBuildInfo` and environment discovery). |
| [`processor.Tag`](../processor/processor.go) | `Monolog\Processor\TagProcessor` | Injects arbitrary fixed key-value tags into `Record.Extra`. |

---

## 2. What Is New in Monogo (Go-Specific Innovations & Extensions)

While Monogo mirrors Monolog's architecture, Go's runtime characteristics (goroutines, static typing, explicit error handling, cloud containerization) require distinct design patterns:

### 1. `Env` & `EnvMap` Processors
- **Status:** **New in Monogo** *(Not in PHP Monolog core)*.
- **What it does:** [`processor.Env(keys...)`](../processor/processor.go) extracts specified environment variables into `Extra["env"]`, while [`processor.EnvMap(mapping)`](../processor/processor.go) maps environment variables directly to top-level keys in `Record.Extra`.
- **Rationale:** In containerized cloud environments (Kubernetes, AWS ECS, GCP Cloud Run), runtime metadata such as `POD_NAME`, `NAMESPACE`, `CLUSTER`, or `DEPLOY_ENV` is injected via environment variables. Providing built-in environment processors enables zero-boilerplate injection of container metadata.

### 2. First-Class `context.Context` Architecture
- **Status:** **New in Monogo** *(Go standard library idiom)*.
- **What it does:**
  - `ctx context.Context` is strictly the mandatory first argument on all `Logger` level methods (`logger.Info(ctx, ...)`), handler operations (`Handler.Handle(ctx, record)`, `BatchHandler.HandleBatch(ctx, records)`), and lifecycle methods (`Handler.Close(ctx)`).
  - Ambient contextual fields can be attached to Go contexts via `monogo.WithContext(ctx, fields)` or `monogo.WithField(ctx, key, value)` and are automatically extracted and merged into log records across goroutines.
  - Unlike common anti-patterns, `context.Context` is **not stored inside the `Record` struct**. `Record` remains a clean, serializable data carrier, while `ctx` travels explicitly through function parameters.

### 3. Bidirectional Standard Library `log/slog` & `zerolog` Adapters
- **Status:** **New in Monogo** *(Go ecosystem interoperability)*.
- **What it does:**
  - `adapter/slogadapter.NewSlogHandler`: Routes Monogo log records to any standard library `slog.Handler`.
  - `adapter/slogadapter.NewMonogoSlogBridge`: Implements `slog.Handler`, allowing standard library `log/slog` calls to be routed through the Monogo processing pipeline.
  - `adapter/zerologadapter.New`: Routes Monogo log records to `rs/zerolog`.
  - Adapters live in isolated subpackages (`adapter/*`) to keep the core `monogo` package dependency-free.

### 4. Concurrent Goroutine Safety & Copy-On-Write Isolation
- **Status:** **New in Monogo** *(Concurrency paradigm difference)*.
- **What it does:**
  - PHP Monolog executes in single-threaded request lifecycles where loggers are instantiated per request. In Go, a single `*monogo.Logger` and its handlers are shared concurrently across thousands of goroutines.
  - All Monogo components are strictly thread-safe using `sync.RWMutex` / `sync.Mutex`.
  - **Copy-On-Write Handler Isolation:** In PHP Monolog, handlers mutate the `LogRecord` directly. In Monogo, when per-handler processors are configured on a handler, `record.Clone()` makes deep copies of `Context` and `Extra` maps before running the handler's processors. Destination-specific mutations (such as masking secrets or adding destination tags) cannot leak to subsequent handlers in the stack or race across goroutines.

### 5. Construction-Time Immutability via Functional Options
- **Status:** **New in Monogo** *(Idiomatic Go configuration)*.
- **What it does:**
  - Replaces PHP Monolog's mutable runtime setters (`$handler->setLevel(...)`, `$handler->setFormatter(...)`) with immutable functional options (`handler.WithBubble`, `handler.WithFormatter`, `handler.WithProcessor`).
  - Guarantees handlers and loggers are immutable after creation, eliminating data races on read/write paths during high-throughput logging.

### 6. In-Memory Auto-Pruning for Long-Running Processes
- **Status:** **New in Monogo** *(Process lifecycle difference)*.
- **What it does:**
  - PHP Monolog's `DeduplicationHandler` wrote state to local disk files because PHP processes die at the end of each HTTP request.
  - Monogo's `Deduplication` handler uses a high-performance in-memory cache with zero-goroutine lazy auto-pruning. It avoids disk I/O, prevents memory leaks in 24/7 services, and supports pluggable custom store backends (`DeduplicationStore`).

### 7. Batch JSON Formatting Modes
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
| **Formatters** | Segregated `Formatter` & `BatchFormatter` (`Line`, `JSON`) | `TextHandler` / `JSONHandler` | `Formatter` (`Text`, `JSON`) | Encoders | Console / JSON |
| **Interoperability** | Bidirectional `slog` bridge + `zerolog` adapter | Native | Via wrappers | Via `zapio` | Native |
