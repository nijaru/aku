# Aku

> [!WARNING]
> In development. Aku is an experimental Go API framework and the repository is still in the design and scaffolding stage. The examples below show the intended direction, not a stable public API you can drop into production today.

Aku is a stdlib-first Go framework for building JSON APIs with typed handlers, automatic request extraction, and a cleaner path to validation, error responses, and OpenAPI generation.

The project goal is simple: keep `net/http` interoperability, borrow the best handler ergonomics from frameworks like Axum and FastAPI, and stay honest about what the standard library already does well.

## Status

- The repo is currently at the initial project setup stage.
- The next implementation milestones are a thin router wrapper and the first generic handler pipeline.
- The README is ahead of the code on purpose so the public direction is clear, but the API is not finalized yet.

If you need a production-ready Go API framework today, use an established option. If you want a Go-native exploration of typed API ergonomics on top of `net/http`, that is what Aku is being built for.

## Install

The module path is:

```text
github.com/nijaru/aku
```

Once the first public package lands, installation will be the usual:

```bash
go get github.com/nijaru/aku
```

## Why Aku

- `net/http` first: standard `http.Handler`, standard middleware patterns, standard `context.Context`
- typed request handling: define input and output as Go structs instead of manually wiring JSON decode and validation everywhere
- API-focused scope: JSON APIs, not server-side HTML or asset pipelines
- low-magic interop: bring your own database layer, auth stack, and surrounding infrastructure
- performance-minded implementation: keep reflection and allocations out of the hot path wherever practical

## Planned API Shape

The intended developer experience looks roughly like this:

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/nijaru/aku"
)

type CreateUserRequest struct {
	Body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
}

type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func CreateUser(ctx context.Context, req CreateUserRequest) (*UserResponse, error) {
	return &UserResponse{
		ID:    "123",
		Name:  req.Body.Name,
		Email: req.Body.Email,
	}, nil
}

func main() {
	app := aku.New()
	aku.Post(app, "/users", CreateUser)
	log.Fatal(http.ListenAndServe(":8080", app))
}
```

That snippet is illustrative only. The core idea is:

- route with the standard library
- describe request data with explicit structs
- let the framework handle extraction and JSON response formatting, with validation hooks fitting into the same pipeline

## Design Goals

- Stay compatible with the Go ecosystem instead of wrapping everything in framework-specific abstractions.
- Make the happy path for JSON APIs feel concise without hiding the underlying HTTP model.
- Keep the public surface small and predictable.
- Generate useful API metadata from explicit handler types rather than handwritten schemas.

## Roadmap

Near-term milestones:

- stdlib-native app/router wrapper
- generic request extraction for body, path, query, and headers
- automatic JSON error responses for bad input
- validation hooks for typed request structs

Planned after that:

- OpenAPI generation from handler types
- built-in API documentation endpoint
- structured logging and core middleware defaults

## Current Repo Layout

- `README.md`: public project overview and roadmap
- `go.mod`: module declaration for `github.com/nijaru/aku`
- `ai/`: local-only design notes, status tracking, and session context

## Development

```bash
go fmt ./...
go test ./...
go build ./...
```

Intended local verification flow:

```bash
go fmt ./... && go test ./... && go build ./...
```
