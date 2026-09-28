# Deliberately Unimplemented Monolog Features

This document details the features and patterns from PHP Monolog (and PSR-3) that are **deliberately omitted** from Monogo (`github.com/githoober/monogo`), along with the architectural and idiomatic Go rationales behind each decision.

---

## B. PSR-3 Message Placeholder Interpolation (`{placeholder}` replacement) — Core Monolog Feature

### What it is in Monolog / PSR-3
In PSR-3 and Monolog, message strings can contain `{key}` placeholders that are automatically parsed and replaced at runtime with matching values from the `context` map:

```php
// PHP Monolog
$logger->info("User {username} created order {order_id}", [
    "username" => "alice",
    "order_id" => 1234,
]);
// Output message: "User alice created order 1234"
```

### Why Deliberately Omitted in Monogo

1. **Modern Structured Logging Philosophy:**
   In cloud-native observability (OpenTelemetry, Google Cloud Logging, Datadog, Elasticsearch, Loki), log messages are intended to be **static, low-cardinality event templates** (e.g. `"User created order"`), while variable data belongs strictly in structured attributes (`username: "alice"`, `order_id: 1234`). Keeping the message template static allows log aggregation backends to index, aggregate, and group occurrences without expensive regex extraction.

2. **Zero Runtime String Scanning Overhead:**
   Scanning every log string for curly braces and performing dynamic map lookups and allocations introduces unnecessary CPU and memory overhead on performance-critical hot paths.

3. **Explicit Formatting:**
   In Go, if a human-readable interpolated string is genuinely required at the call site, standard `fmt.Sprintf` is explicit, familiar, and compiler-checked.

---

## Formatted `*f` Methods (`Logf`, `Infof`, `Errorf`, etc.)

### What it is
Go logging frameworks inspired by `log.Printf` often provide `*f` variants that accept `fmt.Sprintf` format specifiers and variable argument lists (`logger.Logf(ctx, level, "User %s", user)`).

### Why Deliberately Omitted in Monogo

1. **Enforcing Structured Context:**
   `*f` methods encourage developers to bake variables into unstructured strings rather than attaching structured key-value pairs into the log context.
2. **Lean API Surface:**
   Callers who need string interpolation can explicitly pass `fmt.Sprintf(...)` as the message string:
   ```go
   logger.Info(ctx, fmt.Sprintf("Processing batch %d of %d", current, total), fields)
   ```
   Omitting `*f` variants keeps the `Logger` contract clean, consistent, and strictly focused on structured key-value logging.

---

## Duplicated `*Context` Method Sets (`Info` vs `InfoContext`)

### What it is
Go's standard `log/slog` duplicates every logging level into two methods:
- `logger.Info(msg, args...)` (for quick scripts or tests without context)
- `logger.InfoContext(ctx, msg, args...)` (for request-scoped execution)

### Why Deliberately Omitted in Monogo

1. **Strict Context Propagation:**
   Providing parameterless context methods invites developers to bypass `ctx`, dropping ambient fields, request tracing, and deadline propagation across goroutines.
2. **API Bloat:**
   Across Monogo's 8 RFC 5424 levels (`DEBUG` through `EMERGENCY`), supporting duplicate method sets results in 16+ methods on the logger interface.
3. **Idiomatic Go Standard:**
   Following modern Go standards (such as `database/sql`, `net/http`, and modern cloud SDKs), `ctx context.Context` is strictly the mandatory first parameter on all level methods ([`logger.Info(ctx, ...)`](../logger.go), [`logger.Error(ctx, ...)`](../logger.go)).

---

## Storing `context.Context` inside the `Record` Struct

### What it is
Smuggling the client's `context.Context` through the logging pipeline by storing it as a field inside the [`Record`](../record.go) struct (`Record.Ctx`).

### Why Deliberately Omitted in Monogo

1. **Standard Go Anti-Pattern:**
   The official Go Context design rules explicitly state:
   > *"Do not store Contexts inside a struct type; instead, pass a Context explicitly to each function that needs it."*
2. **Pure Data Serialization:**
   `Record` is designed as a pure structured data carrier containing timestamp, level, channel, message, context map, and extra attributes. Storing an interface with channels, mutexes, and timers inside `Record` prevents safe copying, JSON marshaling, and storage.
3. **Explicit Handler Parameters:**
   Instead of storing context in `Record`, context is explicitly passed as the first parameter to handler interfaces:
   ```go
   Handle(ctx context.Context, record Record) error
   HandleBatch(ctx context.Context, records []Record) error
   ```
   This guarantees clean, explicit propagation directly into context-aware sinks (like `log/slog`).

---

## Runtime Mutators & Setters (`setLevel`, `setFormatter`, `pushHandler` at runtime)

### What it is in Monolog
In PHP, loggers and handlers frequently expose mutable setters (`$handler->setLevel(...)`, `$handler->setFormatter(...)`) called after construction.

### Why Deliberately Omitted in Monogo

1. **Concurrency and Race Conditions:**
   PHP runs in a single-threaded request lifecycle where objects are recreated per request. In Go, a single `*monogo.Logger` and its handlers are shared concurrently across thousands of goroutines. Mutable setters introduce data races or require heavy synchronization on read paths.
2. **Construction-Time Functional Options:**
   All configuration is immutably set at construction time via functional options:
   - [`handler.WithBubble(bool)`](../handler/base.go)
   - [`handler.WithFormatter(monogo.Formatter)`](../handler/base.go)
   - [`handler.WithProcessor(...monogo.Processor)`](../handler/base.go)
   - [`handler.WithRotation(...)`](../handler/rotating_file.go)

---

## Deep Object Reflection & Recursive Normalization (`NormalizerFormatter`)

### What it is in Monolog
PHP Monolog features a heavy `NormalizerFormatter` that recursively inspects arbitrary objects, resources, closures, circular references, and private properties via reflection to serialize them into arrays.

### Why Deliberately Omitted in Monogo

1. **Go's Explicit Serialization Contracts:**
   Go emphasizes explicit type behaviors rather than runtime reflection traversal. Types intended for log contexts implement `json.Marshaler`, `encoding.TextMarshaler`, or `fmt.Stringer`.
2. **Predictable Performance:**
   Recursive reflection traversal incurs non-deterministic CPU overhead, memory allocations, and risks infinite loops on cyclic data structures.

---

## Synchronous Remote Transport Sinks (Native Mail, Slack, Webhooks)

### What it is in Monolog
PHP Monolog includes built-in synchronous transport handlers like `NativeMailerHandler`, `PushoverHandler`, and `SlackWebhookHandler`.

### Why Deliberately Omitted in Monogo

1. **Application Latency & Failure Cascades:**
   Performing synchronous network or HTTP calls inside an application logging pipeline blocks request threads and risks cascading timeouts if remote endpoints stall.
2. **Cloud-Native 12-Factor Standard:**
   Modern Go applications follow the 12-Factor App rule: applications stream structured logs to `stdout`/`stderr` or high-performance local forwarders (Vector, Fluent Bit, OpenTelemetry Collector), which handle resilient batch shipping, retries, and network buffering out-of-process.
