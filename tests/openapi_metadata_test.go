package aku_test

import (
	"context"
	"testing"

	"github.com/nijaru/aku"
)

func TestOpenAPIMetadata(t *testing.T) {
	app := aku.New()

	type In struct{}
	type Out struct {
		Message string `json:"message"`
	}

	h := func(ctx context.Context, in In) (Out, error) {
		return Out{Message: "ok"}, nil
	}

	// Normal route
	aku.Get(app, "/normal", h,
		aku.WithSummary("Normal Summary"),
		aku.WithDescription("Normal Description"),
		aku.WithOperationID("normalOp"),
		aku.WithTags("tag1"),
	)

	// Deprecated route
	aku.Get(app, "/deprecated", h,
		aku.WithDeprecated(),
		aku.WithOperationID("depOp"),
	)

	// Internal route (should be hidden)
	aku.Get(app, "/internal", h,
		aku.WithInternal(),
	)

	doc := app.OpenAPIDocument("Test API", "1.0.0")
	
	// Verify normal route
	path, ok := doc.Paths["/normal"]
	if !ok {
		t.Fatal("expected /normal path")
	}
	op := path["get"]
	if op.Summary != "Normal Summary" {
		t.Errorf("expected summary 'Normal Summary', got '%s'", op.Summary)
	}
	if op.Description != "Normal Description" {
		t.Errorf("expected description 'Normal Description', got '%s'", op.Description)
	}
	if op.OperationID != "normalOp" {
		t.Errorf("expected operationId 'normalOp', got '%s'", op.OperationID)
	}
	if len(op.Tags) != 1 || op.Tags[0] != "tag1" {
		t.Errorf("expected tags ['tag1'], got %v", op.Tags)
	}

	// Verify deprecated route
	path, ok = doc.Paths["/deprecated"]
	if !ok {
		t.Fatal("expected /deprecated path")
	}
	op = path["get"]
	if !op.Deprecated {
		t.Error("expected deprecated to be true")
	}
	if op.OperationID != "depOp" {
		t.Errorf("expected operationId 'depOp', got '%s'", op.OperationID)
	}

	// Verify internal route is hidden
	if _, ok := doc.Paths["/internal"]; ok {
		t.Error("expected /internal path to be hidden")
	}
}
