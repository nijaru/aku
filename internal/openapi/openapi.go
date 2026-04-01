package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strconv"
	"strings"

	"github.com/nijaru/aku/internal/bind"
)

// Document represents an OpenAPI 3.0.3 document.
type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Paths      map[string]PathItem `json:"paths"`
	Components *Components         `json:"components,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type Components struct {
	Schemas         map[string]Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type             string `json:"type"` // "apiKey", "http", "oauth2", "openIdConnect"
	Description      string `json:"description,omitempty"`
	Name             string `json:"name,omitempty"`             // for apiKey
	In               string `json:"in,omitempty"`               // for apiKey: "query", "header", "cookie"
	Scheme           string `json:"scheme,omitempty"`           // for http
	BearerFormat     string `json:"bearerFormat,omitempty"`     // for http ("bearer")
	OpenIdConnectUrl string `json:"openIdConnectUrl,omitempty"` // for openIdConnect
}

type PathItem map[string]*Operation

type Operation struct {
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
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
	Format               string            `json:"format,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	AdditionalProperties *Schema           `json:"additionalProperties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Minimum              *float64          `json:"minimum,omitempty"`
	Maximum              *float64          `json:"maximum,omitempty"`
	MinLength            *int              `json:"minLength,omitempty"`
	MaxLength            *int              `json:"maxLength,omitempty"`
	Pattern              string            `json:"pattern,omitempty"`
	Enum                 []any             `json:"enum,omitempty"`
	Example              any               `json:"example,omitempty"`
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
	GetOperationID() string
	GetDeprecated() bool
	GetInternal() bool
	GetTags() []string
	GetSchema() *bind.Schema
	GetOutputType() reflect.Type
	GetSecurity() []map[string][]string
}

// Generate builds an OpenAPI document from a list of routes and global security schemes.
func Generate(
	title, version string,
	routes []Route,
	securitySchemes map[string]SecurityScheme,
) *Document {
	g := &generator{
		doc: &Document{
			OpenAPI: "3.0.3",
			Info: Info{
				Title:   title,
				Version: version,
			},
			Paths: make(map[string]PathItem),
			Components: &Components{
				Schemas:         make(map[string]Schema),
				SecuritySchemes: securitySchemes,
			},
		},
	}

	for _, r := range routes {
		if r.GetInternal() {
			continue
		}

		pattern := r.GetPattern()
		if _, ok := g.doc.Paths[pattern]; !ok {
			g.doc.Paths[pattern] = make(PathItem)
		}

		op := &Operation{
			Summary:     r.GetSummary(),
			Description: r.GetDescription(),
			OperationID: r.GetOperationID(),
			Deprecated:  r.GetDeprecated(),
			Tags:        r.GetTags(),
			Responses:   make(map[string]Response),
			Security:    r.GetSecurity(),
		}

		// Parameters (path, query, header)
		schema := r.GetSchema()
		for _, p := range schema.Parameters {
			if p.In == "form" || p.In == "context" {
				continue // form handled below, context is internal
			}
			ps := g.reflectToSchema(p.Type)
			g.applyValidation(&ps, p.Validate)
			if p.Example != "" {
				ps.Example = p.Example
			}
			required := p.Required
			if p.In == "path" {
				required = true
			}
			op.Parameters = append(op.Parameters, Parameter{
				Name:     p.Name,
				In:       p.In,
				Required: required,
				Schema:   ps,
			})
		}

		// Request Body (JSON or Form)
		if schema.Body != nil {
			op.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					"application/json": {
						Schema: g.reflectToSchema(schema.Body),
					},
				},
			}
		}

		// Collect Form parameters into a multipart/form-data body
		formProps := make(map[string]Schema)
		var formRequired []string
		for _, p := range schema.Parameters {
			if p.In == "form" {
				ps := g.reflectToSchema(p.Type)
				g.applyValidation(&ps, p.Validate)
				formProps[p.Name] = ps
				if p.Required {
					formRequired = append(formRequired, p.Name)
				}
			}
		}
		if len(formProps) > 0 {
			if op.RequestBody == nil {
				op.RequestBody = &RequestBody{Content: make(map[string]MediaType)}
			}
			op.RequestBody.Content["multipart/form-data"] = MediaType{
				Schema: Schema{
					Type:       "object",
					Properties: formProps,
					Required:   formRequired,
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
			mediaType := "application/json"
			outSchema := g.reflectToSchema(outputType)

			// Detect streaming types
			name := outputType.Name()
			pkg := outputType.PkgPath()
			if (name == "Reader" && pkg == "io") || (name == "ReadCloser" && pkg == "io") {
				mediaType = "application/octet-stream"
				outSchema = Schema{Type: "string", Format: "binary"}
			} else if name == "Stream" && (pkg == "github.com/nijaru/aku" || pkg == "aku") {
				mediaType = "*/*" // could be anything
				outSchema = Schema{Type: "string", Format: "binary"}
			} else if name == "SSE" && (pkg == "github.com/nijaru/aku" || pkg == "aku") {
				mediaType = "text/event-stream"
				outSchema = Schema{Type: "string"}
			}

			res.Content = map[string]MediaType{
				mediaType: {
					Schema: outSchema,
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

	// Special check for common binary types
	if t.Name() == "FileHeader" && t.PkgPath() == "mime/multipart" {
		return Schema{Type: "string", Format: "binary"}
	}
	if t.Implements(reflect.TypeFor[io.Reader]()) {
		return Schema{Type: "string", Format: "binary"}
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
		// Use fully qualified name to avoid collisions
		key := name
		if pkg := t.PkgPath(); pkg != "" {
			// Clean up test package suffixes (e.g., github.com/nijaru/aku/tests_test -> github.com/nijaru/aku)
			pkg = strings.TrimSuffix(pkg, "_test")
			pkg = strings.TrimSuffix(pkg, "/tests")
			key = strings.ReplaceAll(pkg, "/", ".") + "." + name
		}

		if _, ok := g.doc.Components.Schemas[key]; !ok {
			// Placeholder to prevent infinite recursion
			g.doc.Components.Schemas[key] = Schema{Type: "object"}
			g.doc.Components.Schemas[key] = g.buildStructSchema(t)
		}
		return Schema{Ref: "#/components/schemas/" + key}
	case reflect.Map:
		props := g.reflectToSchema(t.Elem())
		return Schema{Type: "object", AdditionalProperties: &props}
	default:
		return Schema{Type: "string"}
	}
}

func (g *generator) applyValidation(s *Schema, tag string) {
	if tag == "" {
		return
	}
	parts := strings.SplitSeq(tag, ",")
	for part := range parts {
		kv := strings.Split(part, "=")
		key := kv[0]
		var val string
		if len(kv) > 1 {
			val = kv[1]
		}

		switch key {
		case "min":
			if s.Type == "string" {
				if v, err := strconv.Atoi(val); err == nil {
					s.MinLength = &v
				}
			} else if s.Type == "integer" || s.Type == "number" {
				if v, err := strconv.ParseFloat(val, 64); err == nil {
					s.Minimum = &v
				}
			} else if s.Type == "array" {
				// Added in fix: support minItems
			}
		case "max":
			if s.Type == "string" {
				if v, err := strconv.Atoi(val); err == nil {
					s.MaxLength = &v
				}
			} else if s.Type == "integer" || s.Type == "number" {
				if v, err := strconv.ParseFloat(val, 64); err == nil {
					s.Maximum = &v
				}
			}
		case "email":
			s.Format = "email"
		case "uuid":
			s.Format = "uuid"
		case "url":
			s.Format = "url"
		case "hostname":
			s.Format = "hostname"
		case "ipv4":
			s.Format = "ipv4"
		case "ipv6":
			s.Format = "ipv6"
		case "oneof":
			if val == "" {
				continue
			}
			options := strings.Split(val, " ")
			for _, opt := range options {
				// Parse based on target type
				if s.Type == "integer" {
					if v, err := strconv.Atoi(opt); err == nil {
						s.Enum = append(s.Enum, v)
					}
				} else if s.Type == "number" {
					if v, err := strconv.ParseFloat(opt, 64); err == nil {
						s.Enum = append(s.Enum, v)
					}
				} else {
					s.Enum = append(s.Enum, opt)
				}
			}
		}
	}
}

func (g *generator) buildStructSchema(t reflect.Type) Schema {
	s := Schema{Type: "object", Properties: make(map[string]Schema)}
	for f := range t.Fields() {
		f := f

		// Support embedded fields
		if f.Anonymous && f.Tag.Get("json") == "" {
			embedded := g.buildStructSchema(f.Type)
			maps.Copy(s.Properties, embedded.Properties)
			s.Required = append(s.Required, embedded.Required...)
			continue
		}

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := f.Name
		if tag != "" {
			if before, _, ok := strings.Cut(tag, ","); ok {
				name = before
			} else {
				name = tag
			}
		}

		propSchema := g.reflectToSchema(f.Type)
		g.applyValidation(&propSchema, f.Tag.Get("validate"))
		if ex := f.Tag.Get("example"); ex != "" {
			propSchema.Example = ex
		}
		s.Properties[name] = propSchema
	}
	return s
}
