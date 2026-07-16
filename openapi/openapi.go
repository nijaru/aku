// Package openapi exposes Aku's generated OpenAPI document model.
package openapi

import internal "github.com/nijaru/aku/internal/openapi"

type (
	Document       = internal.Document
	Info           = internal.Info
	Components     = internal.Components
	SecurityScheme = internal.SecurityScheme
	PathItem       = internal.PathItem
	Operation      = internal.Operation
	Parameter      = internal.Parameter
	RequestBody    = internal.RequestBody
	Response       = internal.Response
	MediaType      = internal.MediaType
	Schema         = internal.Schema
	Route          = internal.Route
)

// Generate builds an OpenAPI document from route metadata and security schemes.
func Generate(
	title, version string,
	routes []Route,
	securitySchemes map[string]SecurityScheme,
) *Document {
	return internal.Generate(title, version, routes, securitySchemes)
}
