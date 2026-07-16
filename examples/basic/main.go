package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/middleware"
)

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type CreateProductRequest struct {
	Body struct {
		Name  string  `json:"name" validate:"required"`
		Price float64 `json:"price" validate:"gt=0"`
	}
}

// Explicit validation hook
func (r CreateProductRequest) Validate() error {
	if r.Body.Name == "Forbidden" {
		return errors.New("forbidden name")
	}
	return nil
}

type GetProductRequest struct {
	Path struct {
		ID string `path:"id"`
	}
}

func CreateProduct(ctx context.Context, in CreateProductRequest) (Product, error) {
	return Product{
		ID:    "p_123",
		Name:  in.Body.Name,
		Price: in.Body.Price,
	}, nil
}

func GetProduct(ctx context.Context, in GetProductRequest) (Product, error) {
	return Product{
		ID:    in.Path.ID,
		Name:  "Example Product",
		Price: 99.99,
	}, nil
}

func main() {
	app := aku.New(
		aku.WithValidator(validator.New()),
		aku.WithGlobalMiddleware(
			middleware.BodySizeLimit(middleware.BodySizeLimitConfig{MaxBodyBytes: 1 << 20}),
		),
	)

	// Register routes
	if err := aku.Post(app, "/products", CreateProduct); err != nil {
		log.Fatal(err)
	}
	if err := aku.Get(app, "/products/{id}", GetProduct); err != nil {
		log.Fatal(err)
	}

	// Documentation
	app.OpenAPI("/openapi.json", "Basic Product API", "1.0.0")
	app.SwaggerUI("/docs", "/openapi.json")

	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("API Docs available at http://localhost:8080/docs")
	log.Fatal(http.ListenAndServe(":8080", app))
}
