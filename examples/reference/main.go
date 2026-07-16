// Command reference is a small, runnable Aku application that demonstrates the
// framework's main API-first integration points in one place.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
	"github.com/nijaru/aku/file"
	"github.com/nijaru/aku/middleware"
)

//go:embed web/*
var webFiles embed.FS

type item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type itemRequest struct {
	Auth struct {
		Token auth.Bearer
	}
	Path struct {
		ID string `path:"id"`
	}
	Body struct {
		Name string `json:"name" validate:"required"`
	}
}

type uploadRequest struct {
	Form struct {
		Name  string                `form:"name" validate:"required"`
		Photo *multipart.FileHeader `form:"photo" aku:"required"`
	}
}

type webhookRequest struct {
	Header struct {
		Signature string `header:"X-Signature"`
	}
	Body struct {
		Event string `json:"event" validate:"required"`
	}
}

type uploadResponse struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

func createItem(_ context.Context, in itemRequest) (item, error) {
	return item{ID: in.Path.ID, Name: in.Body.Name}, nil
}

func uploadFile(_ context.Context, in uploadRequest) (uploadResponse, error) {
	uploaded := file.File{Header: in.Form.Photo}
	if err := uploaded.Validate(5<<20, "text/plain; charset=utf-8"); err != nil {
		return uploadResponse{}, err
	}

	contentType, err := uploaded.ContentType()
	if err != nil {
		return uploadResponse{}, err
	}

	return uploadResponse{
		Name:        in.Form.Name,
		Filename:    in.Form.Photo.Filename,
		Size:        in.Form.Photo.Size,
		ContentType: contentType,
	}, nil
}

func receiveWebhook(_ context.Context, in webhookRequest) (map[string]string, error) {
	return map[string]string{
		"event":     in.Body.Event,
		"signature": in.Header.Signature,
	}, nil
}

func main() {
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	checks := middleware.NewHealthChecker()
	checks.Add("database", middleware.HealthyCheck)

	app := aku.New(
		aku.WithValidator(validator.New()),
		aku.WithGlobalMiddleware(
			middleware.Recover,
			middleware.RequestID,
			middleware.Logger,
			middleware.BodySizeLimit(middleware.BodySizeLimitConfig{MaxBodyBytes: 8 << 20}),
		),
	)
	app.Use(checks.Middleware)
	app.AddSecurityScheme("BearerAuth", aku.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	})

	if err := aku.Post(
		app,
		"/api/items/{id}",
		createItem,
		aku.WithStatus(http.StatusCreated),
		aku.WithSummary("Create an item"),
		aku.WithTag("Items"),
		aku.WithSecurityName("BearerAuth"),
	); err != nil {
		log.Fatal(err)
	}
	if err := aku.Post(
		app,
		"/api/uploads",
		uploadFile,
		aku.WithSummary("Upload a text file"),
		aku.WithTag("Uploads"),
	); err != nil {
		log.Fatal(err)
	}
	if err := aku.Post(
		app,
		"/webhooks/provider",
		receiveWebhook,
		aku.WithSummary("Receive a provider webhook"),
		aku.WithTag("Webhooks"),
	); err != nil {
		log.Fatal(err)
	}
	if err := app.HandleHTTP(
		http.MethodGet,
		"/api/version",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintln(w, "aku-reference/1.0")
		}),
		aku.WithSummary("Show the running example version"),
	); err != nil {
		log.Fatal(err)
	}
	if err := app.StaticFS("/web", http.FS(webRoot), aku.WithSPA()); err != nil {
		log.Fatal(err)
	}
	if err := app.OpenAPI("/openapi.json", "Aku Reference API", "1.0.0"); err != nil {
		log.Fatal(err)
	}
	if err := app.SwaggerUI("/docs", "/openapi.json"); err != nil {
		log.Fatal(err)
	}
	if err := app.RedocUI("/redoc", "/openapi.json"); err != nil {
		log.Fatal(err)
	}

	log.Println("Aku reference app listening on http://localhost:8080/web/")
	log.Println("API docs available at http://localhost:8080/docs")
	log.Fatal(app.Run(":8080"))
}
