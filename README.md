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
- **Fluent Testing**: A chainable testing API that makes asserting on complex API behaviors simple.

## Performance

Aku adds minimal overhead over the standard library by pre-calculating extraction logic at startup.

| Framework | Time/op | Allocs/op |
|-----------|---------|-----------|
| `net/http` (manual) | 2124 ns | 36 |
| **Aku** (automatic) | **2334 ns** | **40** |

*Benchmarks performed on Apple M3 Max, performing path/query extraction, JSON decoding, validation, and JSON encoding.*

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

Aku includes a fluent testing API that handles JSON marshaling and assertions.

```go
func TestGreet(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/greet/{name}", Greet)

	aku.Test(t, app).
		Get("/greet/world?shout=true").
		ExpectStatus(200).
		ExpectJSON(GreetResponse{Message: "Hello, world!"})
}
```

## License

MIT
