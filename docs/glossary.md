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
A destination component responsible for receiving a `Record` and outputting or forwarding it.
- **Core Handlers (`github.com/githoober/monogo/handler`)**:
  - `Stream`: Writes to `io.Writer` (console, files).
  - `JSONStream`: Dedicated stream handler preconfigured with JSON formatting (`NewJSONStream` / `NewJSON`).
  - `RotatingFile`: Writes to rotating files using pure standard library (size, daily, backups, gzip compression, max age).
  - `RotatingJSONFile`: Dedicated rotating file handler preconfigured with JSON formatting (`NewRotatingJSONFile` / `NewJSONRotatingFile`).
  - `FingersCrossed`: Buffers logs until triggered by an action level (e.g. `ERROR`).
  - `Test`: Retains records in memory for assertions during tests.
  - `Null`: Consumes and discards records silently.
- **Extension Handlers (`github.com/githoober/monogo/ext/handler`)**:
  - `RotatingFile`: Alternate rotating file handler powered by `lumberjack.v2`.
  - `Buffer`: Buffers records until capacity or flush level.
  - `Deduplication`: Suppresses duplicate log records occurring within a time window.
  - `WhatFailureGroup`: Multiplexes records to multiple handlers while suppressing all errors and panics.
  - `Sampling`: Downsamples log records based on a 1-in-N factor with optional bypass threshold.
  - `Socket`: Streams formatted log records over network sockets (TCP, UDP, Unix domain sockets).
  - `SyslogUdp`: Streams formatted RFC 5424 log entries to remote Syslog servers over UDP sockets.
  - `Filter`: Filters records within a level range.
  - `Group`: Multiplexes records to multiple handlers.
  - `FallbackGroup`: Priority failover across child handlers stopping at first success.

### Processor
A function or component that enriches `Record.Extra` with additional system metadata before formatting and handling.
- **Core Processors (`github.com/githoober/monogo/processor`)**:
  - `ProcessId`: OS process ID (`os.Getpid()`, Monolog `ProcessIdProcessor`, aliased as `processor.Process()`).
  - `Web`: HTTP request attributes (url, client IP, method, server, referrer, user agent; Monolog `WebProcessor`).
  - `Env` / `EnvMap`: Environment variables (convenience extension for containerized/cloud environments).
  - `LoadAverage`: System load averages (1m, 5m, 15m, or all; Monolog `LoadAverageProcessor`).
- **Extension Processors (`github.com/githoober/monogo/ext/processor`)**:
  - `Caller`: File, line, and function caller info (Monolog `IntrospectionProcessor`).
  - `Hostname`: OS hostname (Monolog `HostnameProcessor`).
  - `Memory`: Runtime memory statistics (Monolog `MemoryProcessor` / `MemoryUsageProcessor`).
  - `UID`: Unique invocation request ID (Monolog `UidProcessor`).
  - `Git`: Git commit hash, branch, time, and dirty status (Monolog `GitProcessor`).
  - `Tag`: Fixed key-value tag (Monolog `TagProcessor`).

### Formatter
Transforms a `Record` into a byte slice or string format for output. Examples:
- `Line`: Customizable text line template (Monolog `LineFormatter`).
- `JSON`: JSON payload formatter (Monolog `JsonFormatter`).
- `Logstash`: Logstash Event V1 JSON formatter (Monolog `LogstashFormatter`).
- `Syslog`: RFC 5424 syslog line formatter (Monolog `SyslogFormatter`).
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
Defined by `monogo.Resettable` (`Reset(ctx context.Context) error`). Implemented by loggers, handlers, and processors that maintain internal state across log cycles (such as buffering queues, deduplication window stores, and request UIDs). Calling `logger.Reset(ctx)` establishes a concurrency barrier (waiting for in-flight log writes to finish), ends a log cycle, cascades context-aware resets down through all handlers, per-handler processors, and logger processors, and propagates any reset or flush errors back to the caller. This restores the logging stack to a clean state ready to receive subsequent logs without leaking data between jobs or requests. Essential for long-running Go processes (worker pools, HTTP request lifecycles, and test suites).

### FallbackGroup Handler
A priority failover handler (`handler.FallbackGroup` in `ext/handler`) that attempts to dispatch log records to a slice of child handlers in sequential order. Propagation immediately stops as soon as one handler successfully processes the record. If an individual child handler fails or panics, the failure is caught, an optional callback (`handler.WithFallbackCallback`) is invoked for telemetry, and the handler seamlessly falls back to the next child handler in the chain. If all handlers fail, an aggregated multi-error is returned.

### SyslogUdp Handler
A network handler (`handler.SyslogUdp` in `ext/handler`) that streams log entries over a UDP socket to a remote Syslog daemon. Defaults to the RFC 5424 `Syslog` formatter (`formatter.NewSyslog(appName)`), while allowing custom formatters, facility codes, and bubbling configurations.

### Logstash Formatter
A JSON formatter (`formatter.Logstash` in `formatter`) that transforms log records into Logstash Event V1 JSON format (`@timestamp`, `@version`, `host`, `message`, `channel`, `level`). Supports single and batch NDJSON output modes with zero external dependencies.

### Syslog Formatter
An RFC 5424 syslog formatter (`formatter.Syslog` in `formatter`) that encodes log records into standard syslog lines (`<PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG`) with facility calculation and severity mappings.

### LoadAverage Processor
A system processor (`processor.LoadAverage` in `processor`) that samples operating system load averages (1-minute, 5-minute, 15-minute, or all) and injects them into `Record.Extra["load_average"]` using Go standard library facilities.


