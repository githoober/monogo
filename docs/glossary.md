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

### Processor
A function or component that enriches `Record.Extra` with additional system metadata before formatting and handling. Examples:
- `Caller`: File, line, and function caller info.
- `Hostname`: OS hostname.
- `ProcessId`: OS process ID (`os.Getpid()`).
- `Memory`: Runtime memory statistics.
- `UID`: Unique invocation request ID.
- `Git`: Git commit hash, branch, time, and dirty status.
- `Env` / `EnvMap`: Environment variables.
- `Tag`: Fixed key-value tag.

### Formatter
Transforms a `Record` into a byte slice or string format for output. Examples:
- `Line`: Customizable text line template.
- `JSON`: JSON payload formatter.

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

