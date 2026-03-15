package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/nijaru/aku/internal/bind"
)

// Document represents an OpenAPI 3.0.3 document.
type Document struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Paths      map[string]PathItem   `json:"paths"`
	Components *Components           `json:"components,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty"`
}

type PathItem map[string]*Operation

type Operation struct {
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
}

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Schema   Schema `json:"schema"`
}

type RequestBody struct {
	Content map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema Schema `json:"schema"`
}

type Schema struct {
	Ref                  string            `json:"$ref,omitempty"`
	Type                 string            `json:"type,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	AdditionalProperties *Schema           `json:"additionalProperties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	Required             []string          `json:"required,omitempty"`
}

// JSON returns the JSON representation of the document.
func (d *Document) JSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// Route is the interface required by the generator.
// This decouples the generator from the main aku package if needed, 
// though we usually pass aku.Route.
type Route interface {
	GetMethod() string
	GetPattern() string
	GetStatus() int
	GetSummary() string
	GetDescription() string
	GetTags() []string
	GetSchema() *bind.Schema
	GetOutputType() reflect.Type
}

// Generate builds an OpenAPI document from a list of routes.
func Generate(title, version string, routes []Route) *Document {
	g := &generator{
		doc: &Document{
			OpenAPI: "3.0.3",
			Info: Info{
				Title:   title,
				Version: version,
			},
			Paths:      make(map[string]PathItem),
			Components: &Components{Schemas: make(map[string]Schema)},
		},
	}

	for _, r := range routes {
		pattern := r.GetPattern()
		if _, ok := g.doc.Paths[pattern]; !ok {
			g.doc.Paths[pattern] = make(PathItem)
		}

		op := &Operation{
			Summary:     r.GetSummary(),
			Description: r.GetDescription(),
			Tags:        r.GetTags(),
			Responses:   make(map[string]Response),
		}

		// Parameters
		schema := r.GetSchema()
		for _, p := range schema.Parameters {
			op.Parameters = append(op.Parameters, Parameter{
				Name:     p.Name,
				In:       p.In,
				Required: p.Required,
				Schema:   g.reflectToSchema(p.Type),
			})
		}

		// Request Body
		if schema.Body != nil {
			op.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					"application/json": {
						Schema: g.reflectToSchema(schema.Body),
					},
				},
			}
		}

		// Success Response
		status := r.GetStatus()
		if status == 0 {
			status = 200
		}

		statusStr := fmt.Sprintf("%d", status)
		res := Response{Description: "Success"}

		outputType := r.GetOutputType()
		if status != 204 && outputType != nil {
			res.Content = map[string]MediaType{
				"application/json": {
					Schema: g.reflectToSchema(outputType),
				},
			}
		}
		op.Responses[statusStr] = res

		method := strings.ToLower(r.GetMethod())
		g.doc.Paths[pattern][method] = op
	}

	return g.doc
}

type generator struct {
	doc *Document
}

func (g *generator) reflectToSchema(t reflect.Type) Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Schema{Type: "integer"}
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number"}
	case reflect.Slice:
		items := g.reflectToSchema(t.Elem())
		return Schema{Type: "array", Items: &items}
	case reflect.Struct:
		name := t.Name()
		if name == "" {
			// Anonymous struct, inline it
			return g.buildStructSchema(t)
		}

		// Named struct, move to components
		if _, ok := g.doc.Components.Schemas[name]; !ok {
			// Placeholder to prevent infinite recursion
			g.doc.Components.Schemas[name] = Schema{Type: "object"}
			g.doc.Components.Schemas[name] = g.buildStructSchema(t)
		}
		return Schema{Ref: "#/components/schemas/" + name}
	case reflect.Map:
		props := g.reflectToSchema(t.Elem())
		return Schema{Type: "object", AdditionalProperties: &props}
	default:
		return Schema{Type: "string"}
	}
}

func (g *generator) buildStructSchema(t reflect.Type) Schema {
	s := Schema{Type: "object", Properties: make(map[string]Schema)}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := f.Name
		if tag != "" {
			if idx := strings.Index(tag, ","); idx != -1 {
				name = tag[:idx]
			} else {
				name = tag
			}
		}
		s.Properties[name] = g.reflectToSchema(f.Type)
	}
	return s
}
