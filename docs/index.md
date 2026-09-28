# Monogo Documentation

Welcome to the documentation for **Monogo** (`github.com/githoober/monogo`), a flexible, channel-based structured logging library for Go inspired by PHP Monolog.

## Documentation Structure

- [**Architecture & Design Decisions**](architecture.md): Explains the core design choices, pluggable backend adapters (`log/slog`, `zerolog`), ambient context propagation, handler bubbling, per-handler processors, and batch handling.
- [**Comparison & Go Idioms**](comparison.md): Comparative analysis against PHP Monolog, `slog`, `logrus`, `zap`, `zerolog`, and idiomatic Go design choices.
- [**Deliberately Unimplemented Features**](deliberate_omissions.md): Details features intentionally omitted from Monolog (e.g. PSR-3 placeholder interpolation, `*f` methods, runtime setters) and their Go architectural rationales.
- [**Glossary & Concepts**](glossary.md): Terminology and concepts including Channels, Handlers, Processors, Per-Handler Processors, Formatters, Bubbling, and Batching.

## Quick Overview

Monogo is backend-agnostic and provides a classic Monolog logging pipeline supporting single-record and batch handling, bubbling suppression, and two-tier processor pipelines (logger-level and handler-level):

```
Record -> Logger Processors -> Handlers (IsHandling -> Handler Processors -> Formatter -> Output)
```

For quick start guides and code examples, refer to the project [README.md](../README.md).
