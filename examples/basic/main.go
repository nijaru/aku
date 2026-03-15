package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/nijaru/aku"
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
	app := aku.New()

	// Register routes
	aku.Post(app, "/products", CreateProduct)
	aku.Get(app, "/products/{id}", GetProduct)

	// Documentation
	app.OpenAPI("/openapi.json", "Basic Product API", "1.0.0")
	app.SwaggerUI("/docs", "/openapi.json")

	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("API Docs available at http://localhost:8080/docs")
	log.Fatal(http.ListenAndServe(":8080", app))
}
