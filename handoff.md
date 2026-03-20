# Handoff: Aku Reorganization & Observability

## Status
Session focused on project structural health, Go 1.26 modernization, and production-grade observability. The framework is now modular, benchmarked against `net/http`, and supports standard metrics/health handlers.

## Context
- **Root Decluttered**: Split `app.go` and `router.go` into focused components: `aku.go` (entrypoint), `group.go`, `route.go`, `option.go`, `static.go`, `openapi_handlers.go`, `types.go`, and `problem_details.go`.
- **Package Refactor**: 
    - Moved built-in middlewares to `github.com/nijaru/aku/middleware`.
    - Moved integration tests to `tests/` (black-box testing using `aku_test` package).
- **Observability**: Added `HandleHTTP` and `Metrics` methods to `App` and `Group`. These bypass the framework's JSON error interception and typed extraction pipeline, allowing standard `http.Handler` integration (e.g., Prometheus).
- **Performance**: Pooled `errorInterceptor` using `sync.Pool`, reducing allocations for RFC 9457 error responses.
- **Go 1.26**: Updated `go.mod` and adopted `errors.AsType[T](err)` for type-safe error unwrapping.

## Decisions
- **Renamed Entrypoint to `aku.go`**: Standard Go library convention to have the primary file match the package or be clearly identifiable.
- **Renamed to `problem_details.go`**: More descriptive than `problem.go` and avoids generic naming; implements RFC 9457.
- **Flat Public API via Root Files**: Kept core types in the root `package aku` to maintain a simple import path (`github.com/nijaru/aku`) while using multiple files to organize the implementation.
- **Standard Handler Escape Hatch**: `HandleHTTP` allows raw `http.Handler` registration that specifically escapes the framework's middleware/interceptor stack for health/metrics paths.

## Next Steps
- **OpenAPI Caching**: Implement caching for the generated OpenAPI document to avoid re-generating on every request.
- **Strict Extraction**: Add `aku-strict` mode to return 400 if unknown query/header parameters are provided.
- **Compression**: Implement `aku-compress` middleware for Gzip/Brotli support.
- **OpenAPI Metadata**: Enhance generated spec with Operation IDs, Deprecation flags, and common response headers.

## Environment
- **Go Version**: 1.26.1
- **Task**: `aku-uh40` (Declutter and modularize root directory) is marked as `started`.
- **Benchmarks**: Use `b.Loop()` (Go 1.24+ feature) for timer management.
