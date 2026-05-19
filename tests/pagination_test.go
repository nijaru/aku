package aku_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
	"github.com/nijaru/aku/pagination"
)

type pageInput struct {
	Query pagination.Page
}

func TestPaginationPageInput_DefaultsWhenQueryMissing(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/items", func(ctx context.Context, in pageInput) (map[string]int, error) {
		offset, limit := in.Query.Params()
		return map[string]int{"offset": offset, "limit": limit}, nil
	})

	testutil.Test(t, app).
		Get("/items").
		ExpectStatus(http.StatusOK).
		ExpectJSON(map[string]int{"offset": 0, "limit": 20})
}

func TestPaginationPageInput_BindsProvidedQuery(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/items", func(ctx context.Context, in pageInput) (map[string]int, error) {
		offset, limit := in.Query.Params()
		return map[string]int{"offset": offset, "limit": limit}, nil
	})

	testutil.Test(t, app).
		Get("/items?offset=40&limit=10").
		ExpectStatus(http.StatusOK).
		ExpectJSON(map[string]int{"offset": 40, "limit": 10})
}
