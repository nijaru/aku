# Aku

> [!IMPORTANT]
> **Aku** is a high-performance, typesafe web framework for the latest stable Go release, currently Go 1.26.5, designed specifically for building APIs.

Aku bridges the gap between the standard library's `net/http` and the ergonomics of modern frameworks like FastAPI or Axum. It uses Go's type system to automate request extraction, validation, and documentation without sacrificing compatibility.

## Features

- **Standard Library First**: Built on `net/http` and Go's modern `http.ServeMux`. Pure `http.Handler` compatibility.
- **Typesafe Extraction**: Automatically map Path, Query, Header, Form, and Body into a single request struct.
- **Precompiled Extraction Plans**: Reflection inspects handler types at registration; request handling reuses the resulting binding plan and coercers.
- **Automatic OpenAPI 3.0**: Generates documentation, including schemas and security requirements, from your Go types.
- **Validation**: Support for `go-playground/validator` tags (opt in with `WithValidator`) and explicit `Validate() error` hooks.
- **Streaming & SSE**: First-class support for `io.Reader` streaming and Server-Sent Events.
- **Middleware Suite**: Production-ready `Recover`, `Timeout`, and `CORS` implementations.
- **Integration Testing**: The repo's integration suite uses chainable helpers to keep API assertions readable.

## Performance

Aku is designed with a **low-allocation philosophy**. By inspecting Go types at route registration time, Aku precomputes extraction steps and coercers for every handler. At runtime, the request path reuses those plans and pooled input values. Field assignment still uses `reflect.Value`; the optimization is avoiding repeated type inspection and coercer construction, not code-generated zero-reflection execution.

- **Pre-compiled Binding**: Avoids repeated reflection-based type inspection and conversion setup on each request.
- **Buffer & Struct Reuse**: Extensive use of `sync.Pool` to minimize GC pressure.
- **Standard Library Speed**: Built directly on `http.ServeMux` with minimal wrapping.

*Benchmark results are environment-sensitive; see `benchmark_test.go` for the comparison suite and rerun it on your target hardware.*

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
	if err := aku.Get(app, "/greet/{name}", Greet); err != nil {
		log.Fatal(err)
	}

	// Serve OpenAPI UI at /docs
	app.OpenAPI("/openapi.json", "My API", "1.0.0")
	app.SwaggerUI("/docs", "/openapi.json")

	log.Println("Serving on :8080")
	log.Fatal(http.ListenAndServe(":8080", app))
}
```

## Development

Aku targets the latest stable Go toolchain. The module currently requires Go
1.26.5 so the framework can use current standard-library APIs and runtime
improvements. Some underlying HTTP semantics come from Go 1.22-era `ServeMux`,
but older toolchains are not a supported build target.

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

Non-pointer query, header, and form fields are required by default. Add
`aku:"optional"` when a zero value is meaningful and your handler applies
defaults:

```go
type ListProductsRequest struct {
	Query struct {
		Offset int `query:"offset" aku:"optional"`
		Limit  int `query:"limit"  aku:"optional"`
	}
}
```

Validation tags are enforced when the application is configured with a validator;
explicit `Validate() error` hooks run without that option:

```go
app := aku.New(aku.WithValidator(validator.New()))
```

Typed JSON and form bodies are not size-limited by default because payload budgets
are application-specific. Add `middleware.BodySizeLimit` before serving routes that
accept request bodies.

## Middleware

Use standard `func(http.Handler) http.Handler` middleware at the application or route level.

```go
app := aku.New(
    aku.WithGlobalMiddleware(middleware.Recover, middleware.Logger),
)

aku.Post(app, "/secure", MyHandler, 
    aku.WithMiddleware(AuthMiddleware),
)
```

## Standard Handler Escape Hatches

Aku keeps `http.Handler` compatibility for endpoints that do not need a typed request struct.
`HandleHTTP` registers any standard handler with route metadata and middleware, while `Metrics`
is a shorthand for read-only GET endpoints such as `/metrics`.

```go
app.HandleHTTP(
    http.MethodGet,
    "/healthz",
    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNoContent)
    }),
    aku.WithSummary("Health check"),
)

app.Metrics("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}))
```

These routes still participate in application, group, and route middleware, and they still feed
OpenAPI metadata.

Typed registration returns an error when the input contract or `ServeMux` pattern is invalid;
check those errors during startup. Static file routes are intentionally not included in OpenAPI.

## Server Runtime

`app.Run(addr)` creates a standard `http.Server` with bounded defaults:
`ReadHeaderTimeout=5s`, `ReadTimeout=30s`, `WriteTimeout=30s`, and
`IdleTimeout=120s`. For long-lived streaming endpoints, set the write timeout
to zero or construct your own `http.Server` with `app` as the handler.

```go
app := aku.New(aku.WithServerTimeouts(aku.ServerTimeouts{
	ReadHeader: 5 * time.Second,
	Read:       30 * time.Second,
	Write:      0, // allow long-lived streams
	Idle:       120 * time.Second,
}))
```

## Browser Applications

Aku stays API-first. Browser-facing concerns such as cookie sessions, CSRF,
HTML rendering, and template engines should be composed as standard
`net/http` middleware or `http.Handler` routes through `Use`, `WithMiddleware`,
and `HandleHTTP`. Aku will not force a session store, CSRF library, template
engine, or application architecture.

## Testing

Aku's integration tests live in `tests/` and use repo-local helpers in
`internal/testutil` to keep assertions concise.

That helper stays internal for now; the public API is the framework itself, not
another testing abstraction. If you need to write your own Aku tests, use
`httptest` directly or copy the patterns that fit your app.

## License

MIT
