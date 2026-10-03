# Comparison & Idiomatic Go Design

This document provides a comparative analysis of **Monogo** (`github.com/githoober/monogo`) against PHP Monolog, other popular Go logging frameworks (`sirupsen/logrus`, `log/slog`, `uber-go/zap`, `rs/zerolog`), and explains key idiomatic Go design choices.

## 1. Monogo vs. PHP Monolog

| Feature / Pattern | PHP Monolog | Monogo | Rationale |
| :--- | :--- | :--- | :--- |
| **Type System & OOP** | Classes, Interfaces, Inheritance | Structs, Small Interfaces, Embedding | Go uses composition and interfaces rather than deep class hierarchies. |
| **Error Handling** | Exceptions (`throw \Exception`) | Explicit `error` return values | Idiomatic Go error handling. |
| **Call-site Fields** | Array `['user' => 42]` | Variadic `...map[string]interface{}` & `context.Context` | Supports both in-place map literals and Go's `context.Context`. |
| **Processors** | Callable `function(LogRecord $r)` | `Processor` interface & `ProcessorFunc` | Mirrors Go's standard `http.Handler` / `http.HandlerFunc` pattern. |
| **Per-Handler Processors** | `ProcessableHandlerInterface` (`pushProcessor`) | `ProcessableHandler` & `handler.WithProcessor(...)` | Configured at construction via functional options; executes handler-specific processors and clones records to prevent mutation leakage. |
| **Bubbling** | `$bubble = false` halts propagation | `Bubbler` interface & `handler.WithBubble(bool)` option | Configured at construction time via options; stops record propagation down the handler stack when `Bubble()` returns `false`. |
| **Batch Handling** | `handleBatch(array $records)` / `formatBatch` | `BatchHandler` / `BatchFormatter` interfaces | Optional segregated interfaces (`BatchHandler`, `BatchFormatter`) avoid forcing iteration boilerplate on simple handlers while enabling atomic flushes and bulk formatting where supported. |
| **Concurrency** | Single-threaded PHP execution | Thread-safe (`sync.RWMutex` / `sync.Mutex`) | Safely usable across concurrent goroutines in Go web servers/workers. |

---

## 2. Comparison with Popular Go Loggers

| Feature / Concept | PHP Monolog / Monogo | `sirupsen/logrus` | `log/slog` (Stdlib) | `uber-go/zap` | `rs/zerolog` |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Channel Support** | First-class (`Record.Channel`) | Not built-in | Not built-in | Logger name | Logger component |
| **Call-site Fields** | `Context` map | `WithFields` | `Attr` / variadic | Typed `Field`s | Chained `Fields` |
| **System Metadata** | `Extra` map (via Processors) | Hooks | Handler wrappers | Core / Encoders | Event hooks |
| **Handler Pipeline** | Handlers stack (`FingersCrossed`, `Buffer`, `Stream`) | Hooks / `io.Writer` | `slog.Handler` | `zapcore.Core` | `io.Writer` |
| **Formatters** | `Formatter` (`Line`, `JSON`) | `Formatter` | Text / JSON | Encoders | Console / JSON |

---

## 3. Idiomatic Go Design Highlights

### Interface Segregation
Core components are defined as small, focused Go interfaces (`Handler`, `Processor`, `Formatter`). Shared behavior across handlers is achieved via struct embedding (`handler.BaseHandler`).

### Zero External Dependencies in Core
The root `monogo` package relies exclusively on the Go standard library. Third-party or framework-specific integrations reside in isolated subpackages (`adapter/slogadapter`, `adapter/zerologadapter`), ensuring consumers importing core Monogo pull in zero unwanted dependencies.

### `context.Context` Ambient Field Propagation
Monogo integrates directly with Go's standard `context.Context` (`monogo.WithField`, `monogo.WithContext`, `monogo.FromContext`). Contextual fields travel implicitly across API boundaries and goroutines and are automatically merged into log records across all Logger level methods.

### Bidirectional `log/slog` Compatibility
Monogo provides full bidirectional interoperability with Go 1.21+ `log/slog`:
- Send Monogo records to any `slog.Handler` via `slogadapter.NewSlogHandler`.
- Route standard library `slog` calls through Monogo via `slogadapter.NewMonogoSlogBridge`.
