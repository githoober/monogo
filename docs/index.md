# Monogo Documentation

Welcome to the documentation for **Monogo** (`github.com/githoober/monogo`), a flexible, channel-based structured logging library for Go inspired by PHP Monolog.

## Documentation Structure

- [**Architecture & Design Decisions**](architecture.md): Explains the core design choices, pluggable backend adapters (`log/slog`, `zerolog`), ambient context propagation, and handler pipeline.
- [**Comparison & Go Idioms**](comparison.md): Comparative analysis against PHP Monolog, `slog`, `logrus`, `zap`, `zerolog`, and idiomatic Go design choices.
- [**Glossary & Concepts**](glossary.md): Terminology and concepts including Channels, Handlers, Processors, Formatters, and Log Levels.

## Quick Overview

Monogo is backend-agnostic and provides a classic Monolog logging pipeline:

Record -> Processors -> Handlers -> Formatter -> Output (Stream, File, Slog, Zerolog)


For quick start guides and code examples, refer to the project [README.md](../README.md).
