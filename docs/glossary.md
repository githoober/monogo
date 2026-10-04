# Glossary & Key Concepts

This document explains key terms and components used throughout **Monogo**.

## Core Terminology

### Channel
A string identifier categorizing where a log record originated from (e.g., `app`, `auth`, `database`, `api`). Channels allow routing logs to different handlers or filtering outputs.

### Record
The central data structure passed through the logging pipeline containing:
- `Message`: Main log message string.
- `Level`: RFC 5424 severity level.
- `Channel`: Channel name string.
- `Time`: Timestamp when log entry was generated.
- `Context`: Map of contextual key-value pairs (including ambient context fields).
- `Extra`: Map of metadata added by Processors.

### Handler
A destination component responsible for receiving a `Record` and outputting or forwarding it. Examples:
- `Stream`: Writes to `io.Writer` (console, files).
- `RotatingFile`: Rotates log files based on size/age using `lumberjack`.
- `FingersCrossed`: Buffers logs until triggered by an action level (e.g. `ERROR`).
- `Filter`: Filters records within a level range.
- `Group`: Multiplexes records to multiple handlers.
- `Buffer`: Buffers records until capacity or flush level.
- `Deduplication`: Suppresses duplicate log records occurring within a time window.
- `WhatFailureGroup`: Multiplexes records to multiple handlers while suppressing all errors and panics.

### Processor
A function or component that enriches `Record.Extra` with additional system metadata before formatting and handling. Examples:
- `Caller`: File, line, and function caller info (Monolog `IntrospectionProcessor`).
- `Hostname`: OS hostname (Monolog `HostnameProcessor`).
- `ProcessId`: OS process ID (`os.Getpid()`, Monolog `ProcessIdProcessor`).
- `Memory`: Runtime memory statistics (Monolog `MemoryProcessor` / `MemoryUsageProcessor`).
- `UID`: Unique invocation request ID (Monolog `UidProcessor`).
- `Git`: Git commit hash, branch, time, and dirty status (Monolog `GitProcessor`).
- `Tag`: Fixed key-value tag (Monolog `TagProcessor`).
- `Env` / `EnvMap`: Environment variables (Monogo extension for containerized/cloud environments).

### Formatter
Transforms a `Record` into a byte slice or string format for output. Examples:
- `Line`: Customizable text line template (Monolog `LineFormatter`).
- `JSON`: JSON payload formatter (Monolog `JsonFormatter`).
- `Logfmt`: Canonical key=value logfmt formatter with configurable field names, prefixes, and safe quoting (Monogo extension for Go cloud ecosystems like Loki and Promtail).

### Ambient Context
Contextual key-value pairs stored in Go's `context.Context` (via `monogo.WithContext` / `monogo.WithField`) that are automatically extracted and attached to log entries by all Logger level methods.

### Batch Handling
Supported via `monogo.BatchHandler` (`HandleBatch(ctx context.Context, records []Record) error`) and `monogo.BatchFormatter` (`FormatBatch(records []Record) ([]byte, error)`). Buffering handlers (`Buffer`, `FingersCrossed`) detect `BatchHandler` via type assertion to flush accumulated records in atomic, single-write operations, while cleanly falling back to `Handle` for standard handlers.

### Batch Mode
Formatting options on `formatter.JSON` for multi-record batches:
- `BatchModeNewlines` (default): Formats each record as an independent line of JSON (NDJSON).
- `BatchModeJSON`: Formats the entire batch as a single JSON array (`[...]`).

### Processable Handler
Defined by `monogo.ProcessableHandler` (`Processors() []Processor`, `ProcessRecord(Record) Record`). Enables handlers to have dedicated processors configured via `handler.WithProcessor(...)` that execute before formatting/handling. Clones records when processors are present to prevent mutation leakage to subsequent handlers in the stack.

### Deduplication Handler
A flood-control decorator handler (`handler.Deduplication`) that suppresses identical log records repeating within a configurable time window (`time.Duration`). Only records with `Level >= dedupLevel` (default `ERROR`) are deduplicated, while records below the threshold pass through unconditionally. Provides thread-safe, zero-goroutine auto-pruned in-memory storage and supports custom `DeduplicationStore` backends and custom key extractors (`handler.WithDeduplicationKey`).

### WhatFailureGroup Handler
A resilient multiplexing handler (`handler.WhatFailureGroup`) that forwards log records to a slice of handlers while swallowing and suppressing any errors or recovered panics returned by individual handlers during `Handle`, `HandleBatch`, or `Close`. Always returns `nil` from operations to guarantee that unreliable secondary logging sinks (e.g. webhooks, Slack alerts, Elasticsearch) do not interrupt primary logging or cause application errors. Supports an optional callback (`handler.WithWhatFailureCallback`) for metrics and telemetry without failing the caller.

### Logfmt Formatter
A structured formatter (`formatter.Logfmt`) that serializes log records into standard `key=value` logfmt lines (standard for Grafana Loki, Promtail, Heroku, and Go CLI conventions). Supports configurable field keys (`WithTimeKey`, `WithLevelKey`, `WithChannelKey`, `WithMessageKey`), prefixes for context and extra maps (`WithContextPrefix`, `WithExtraPrefix`), custom timestamp layouts, and canonical quoting rules for values containing whitespace, quotes, or delimiter characters. Fully implements `monogo.BatchFormatter`.

### Resettable Interface
Defined by `monogo.Resettable` (`Reset()`). Implemented by loggers, handlers, and processors that maintain internal state across log cycles (such as buffering queues, deduplication window stores, and request UIDs). Calling `logger.Reset()` ends a log cycle and cascades down through all handlers, per-handler processors, and logger processors, restoring them to a clean state ready to receive subsequent logs without leaking data between jobs or requests. Essential for long-running Go processes (worker pools, HTTP request lifecycles, and test suites).


