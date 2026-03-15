package aku_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/nijaru/aku"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestTester(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/users/{id}", func(ctx context.Context, in struct {
		Path struct {
			ID int `path:"id"`
		}
	}) (User, error) {
		return User{ID: in.Path.ID, Name: "Alice"}, nil
	})

	aku.Post(app, "/users", func(ctx context.Context, in struct {
		Body User
	}) (User, error) {
		return in.Body, nil
	}, aku.WithStatus(http.StatusCreated))

	at := aku.Test(t, app)

	// Test GET
	at.Get("/users/123").
		ExpectStatus(http.StatusOK).
		ExpectJSON(User{ID: 123, Name: "Alice"})

	// Test POST
	newUser := User{ID: 456, Name: "Bob"}
	at.Post("/users").
		WithJSON(newUser).
		ExpectStatus(http.StatusCreated).
		ExpectJSON(newUser)

	// Test 404
	at.Get("/not-found").
		ExpectStatus(http.StatusNotFound)
}
