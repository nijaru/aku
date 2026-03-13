# Aku

High-performance, idiomatic Go web framework designed specifically for building APIs.

## Project Structure

| Directory | Purpose |
| --------- | ------- |
| `docs/`   | User/team documentation |
| `ai/`     | Local-only AI session context excluded via `.git/info/exclude` |
| `.tasks/` | Local-only task tracker state excluded via `.git/info/exclude` |

### AI Context Organization

**Purpose:** Keep project state between sessions without polluting public git history.

**Session files** (local only):

- `ai/STATUS.md` - current state, blockers, active work
- `ai/DESIGN.md` - architecture and generic handler design
- `ai/DECISIONS.md` - append-only design decisions

**Reference files** (local only):

- `ai/research/` - external research and comparisons
- `ai/design/` - deeper component specs
- `ai/tmp/` - scratch artifacts

**Task tracking:** `tk` CLI with `.tasks/` kept local-only. Use `tk ready` to find pending work.

## Technology Stack

| Component | Technology |
| --------- | ---------- |
| Language | Go 1.22+ |
| Module path | `github.com/nijaru/aku` |
| First public package | `github.com/nijaru/aku` |
| HTTP | `net/http` (Go 1.22 ServeMux) |
| Testing | `go test` |
| Formatting | `go fmt` |

## Commands

```bash
# Format
go fmt ./...

# Test
go test ./...

# Build
go build ./...

# Tidy module metadata
go mod tidy
```

## Verification Steps

Commands that should pass before shipping:

- Build: `go build ./...`
- Tests: `go test ./...`
- Format: `go fmt ./...`

## Code Standards

| Aspect | Standard |
| ------ | -------- |
| Base | Idiomatic Go, adhering to standard `net/http` patterns |
| Performance | Zero-allocation reflection where possible, `sync.Pool` |
| Extensibility | Do not force ORMs or Auth. Standard `http.Handler` everywhere |
| Errors | Return properly formatted JSON errors automatically |

## Examples

See `README.md` for conceptual handler examples using Generics.

## Development Workflow

1. Research best practices against Axum, FastAPI, or Huma before implementing the generic layer.
2. Record design decisions in local `ai/` files before broadening the surface.
3. Implement core features iteratively: router, generic extractors, OpenAPI generation.
4. Keep `README.md` up to date as capabilities and examples change.
5. Run `go test ./...` and `go build ./...`.
6. Update local `ai/STATUS.md` and task logs with what changed.

## Current Focus

See local `ai/STATUS.md` for active work and `ai/DESIGN.md` for architecture.