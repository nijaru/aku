# Aku

> [!IMPORTANT]
> **Aku** is a high-performance, typesafe web framework for Go 1.22+ designed specifically for building APIs.

Aku bridges the gap between the standard library's `net/http` and the ergonomics of modern frameworks like FastAPI or Axum. It uses Go's type system to automate request extraction, validation, and documentation without sacrificing compatibility.

## Features

- **Standard Library First**: Built on Go 1.22+ `http.ServeMux`. Pure `http.Handler` compatibility.
- **Typesafe Extraction**: Automatically map Path, Query, Header, Form, and Body into a single request struct.
- **Zero-Reflect Hot Path**: Reflection is used at registration time to "compile" extraction plans; the request path is optimized for performance.
- **Automatic OpenAPI 3.0**: Generates documentation, including schemas and security requirements, from your Go types.
- **Validation**: Built-in support for `go-playground/validator` tags and explicit `Validate() error` hooks.
- **Streaming & SSE**: First-class support for `io.Reader` streaming and Server-Sent Events.
- **Middleware Suite**: Production-ready `Recover`, `Timeout`, and `CORS` implementations.
- **Integration Testing**: The repo's integration suite uses chainable helpers to keep API assertions readable.

## Performance

Aku is designed with a **Zero-Allocation Philosophy**. By leveraging Go's reflection only at route registration time, Aku "compiles" static extraction plans for every handler. At runtime, the request path uses these pre-compiled plans and `sync.Pool` to achieve performance that is nearly identical to hand-optimized `net/http` code.

- **Pre-compiled Extraction**: No reflection in the request hot path.
- **Buffer & Struct Reuse**: Extensive use of `sync.Pool` to minimize GC pressure.
- **Standard Library Speed**: Built directly on `http.ServeMux` with minimal wrapping.

*Benchmarks consistently show Aku adds minimal overhead (~100ns) compared to manually binding `net/http` handlers. See `benchmark_test.go` for the latest verified results.*

## Quick Start

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/nijaru/aku"
)

type GreetRequest struct {
	Path struct {
		Name string `path:"name"`
	}
	Query struct {
		Shout bool `query:"shout"`
	}
}

type GreetResponse struct {
	Message string `json:"message"`
}

func Greet(ctx context.Context, in GreetRequest) (GreetResponse, error) {
	msg := "Hello, " + in.Path.Name
	if in.Query.Shout {
		msg += "!"
	}
	return GreetResponse{Message: msg}, nil
}

func main() {
	app := aku.New()

	// Register a route
	aku.Get(app, "/greet/{name}", Greet)

	// Serve OpenAPI UI at /docs
	app.OpenAPI("/openapi.json", "My API", "1.0.0")
	app.SwaggerUI("/docs", "/openapi.json")

	log.Println("Serving on :8080")
	log.Fatal(http.ListenAndServe(":8080", app))
}
```

## Development

Use the repo-local hook setup to keep Go formatting out of commits:

```bash
make hooks
```

That configures Git to run `.githooks/pre-commit`, which formats staged Go
files before the commit is created.

Useful local checks:

- `make fmt` to rewrite tracked Go files in place
- `make fmt-check` to verify formatting without changing files
- `make check` to run formatting, tests, and the build

Aku also ships a minimal project scaffold generator for existing Go modules:

```bash
go run ./cmd/aku init --dir . --name api
```

It writes a conventional `cmd/<name>/main.go` plus `internal/app` layout without
forcing extra framework opinions.

## Request Extraction

Aku uses struct sections to define where data comes from. Each section is optional.

```go
type CreateProductRequest struct {
	Header struct {
		IDPToken string `header:"X-IDP-Token"`
	}
	Path struct {
		Category string `path:"category"`
	}
	Query struct {
		Preview bool `query:"preview"`
	}
	Body struct {
		Name  string  `json:"name" validate:"required"`
		Price float64 `json:"price" validate:"gt=0"`
	}
}
```

## Middleware

Use standard `func(http.Handler) http.Handler` middleware at the application or route level.

```go
app := aku.New(
    aku.WithMiddleware(middleware.Recover, middleware.Logger),
)

aku.Post(app, "/secure", MyHandler, 
    aku.WithMiddleware(AuthMiddleware),
)
```

## Testing

Aku's integration tests live in `tests/` and use repo-local helpers to keep
assertions concise. A public fluent test DSL is still on the roadmap.

## License

MIT
