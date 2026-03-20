package aku_test

import (
	"context"
	"testing"

	"github.com/nijaru/aku"
)

func TestOpenAPI_Security(t *testing.T) {
	app := aku.New()

	// Add global security schemes
	app.AddSecurityScheme("BearerAuth", aku.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	})
	app.AddSecurityScheme("ApiKeyAuth", aku.SecurityScheme{
		Type: "apiKey",
		In:   "header",
		Name: "X-API-Key",
	})

	// Register route with security
	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "secure", nil
	}, aku.WithSecurity(map[string][]string{"BearerAuth": {}}))

	aku.Get(app, "/multi-secure", func(ctx context.Context, in struct{}) (string, error) {
		return "multi", nil
	}, aku.WithSecurity(map[string][]string{"BearerAuth": {}, "ApiKeyAuth": {}}))

	doc := app.OpenAPIDocument("Security API", "1.0.0")

	// Verify Security Schemes
	if _, ok := doc.Components.SecuritySchemes["BearerAuth"]; !ok {
		t.Fatal("expected BearerAuth security scheme")
	}
	if doc.Components.SecuritySchemes["BearerAuth"].Type != "http" {
		t.Errorf("expected http type, got %s", doc.Components.SecuritySchemes["BearerAuth"].Type)
	}

	// Verify route security
	path := doc.Paths["/secure"]["get"]
	if len(path.Security) != 1 {
		t.Fatalf("expected 1 security requirement, got %d", len(path.Security))
	}
	if _, ok := path.Security[0]["BearerAuth"]; !ok {
		t.Error("expected BearerAuth requirement for /secure")
	}

	multiPath := doc.Paths["/multi-secure"]["get"]
	if len(multiPath.Security) != 1 {
		t.Fatalf("expected 1 security requirement (combined), got %d", len(multiPath.Security))
	}
	if _, ok := multiPath.Security[0]["BearerAuth"]; !ok {
		t.Error("expected BearerAuth in multi-secure")
	}
	if _, ok := multiPath.Security[0]["ApiKeyAuth"]; !ok {
		t.Error("expected ApiKeyAuth in multi-secure")
	}
}
