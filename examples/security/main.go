package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
)

type SecureResponse struct {
	Message string `json:"message"`
}

func SecureHandler(ctx context.Context, _ struct{}) (SecureResponse, error) {
	return SecureResponse{Message: "You are authorized!"}, nil
}

func main() {
	app := aku.New()

	// Define security scheme for OpenAPI
	app.AddSecurityScheme("BearerAuth", aku.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	})

	// Apply security metadata to route
	if err := aku.Get(
		app, "/secure", SecureHandler,
		aku.WithMiddleware(auth.RequireBearer()),
		aku.WithSecurityName("BearerAuth"),
		aku.WithSummary("A secure endpoint"),
		aku.WithTag("Authentication"),
	); err != nil {
		log.Fatal(err)
	}

	if err := app.OpenAPI("/openapi.json", "Secure API", "1.0.0"); err != nil {
		log.Fatal(err)
	}
	if err := app.SwaggerUI("/docs", "/openapi.json"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("Check the documentation for security requirements at http://localhost:8080/docs")
	log.Fatal(http.ListenAndServe(":8080", app))
}
