package aku_test

import (
	"context"
	"testing"

	"github.com/nijaru/aku"
)

type Node struct {
	ID   int   `json:"id"`
	Next *Node `json:"next"`
}

func GetNode(ctx context.Context, in struct{}) (*Node, error) {
	return &Node{ID: 1}, nil
}

func TestOpenAPI_Recursion(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/node", GetNode)

	doc := app.OpenAPI("Recursion API", "1.0.0")

	// Check components
	key := "github.com.nijaru.aku_test.Node"
	nodeSchema, ok := doc.Components.Schemas[key]
	if !ok {
		t.Fatalf("expected Node schema in components at key %q", key)
	}

	if nodeSchema.Type != "object" {
		t.Errorf("expected Node to be object, got %s", nodeSchema.Type)
	}

	nextField, ok := nodeSchema.Properties["next"]
	if !ok {
		t.Fatal("expected 'next' field in Node schema")
	}

	if nextField.Ref != "#/components/schemas/"+key {
		t.Errorf("expected recursive ref #/components/schemas/%s, got %q", key, nextField.Ref)
	}
}
