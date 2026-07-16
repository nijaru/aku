package aku

import (
	"net/http"
	"reflect"
	"slices"
)

// Route represents a registered route and its metadata.
type Route struct {
	Method      string
	Pattern     string
	Status      int
	Summary     string
	Description string
	OperationID string
	Deprecated  bool
	Internal    bool
	Tags        []string
	Security    []map[string][]string
	Schema      *RequestSchema
	OutputType  reflect.Type
	middleware  []func(http.Handler) http.Handler
}

func (r *Route) GetMethod() string      { return r.Method }
func (r *Route) GetPattern() string     { return r.Pattern }
func (r *Route) GetStatus() int         { return r.Status }
func (r *Route) GetSummary() string     { return r.Summary }
func (r *Route) GetDescription() string { return r.Description }
func (r *Route) GetOperationID() string { return r.OperationID }
func (r *Route) GetDeprecated() bool    { return r.Deprecated }
func (r *Route) GetInternal() bool      { return r.Internal }
func (r *Route) GetTags() []string      { return slices.Clone(r.Tags) }
func (r *Route) GetSecurity() []map[string][]string {
	return cloneSecurity(r.Security)
}
func (r *Route) GetSchema() *RequestSchema   { return r.Schema }
func (r *Route) GetOutputType() reflect.Type { return r.OutputType }

func cloneSecurity(security []map[string][]string) []map[string][]string {
	if security == nil {
		return nil
	}
	clone := make([]map[string][]string, len(security))
	for i, requirement := range security {
		clone[i] = make(map[string][]string, len(requirement))
		for name, scopes := range requirement {
			clone[i][name] = slices.Clone(scopes)
		}
	}
	return clone
}

func cloneRoute(route *Route) *Route {
	if route == nil {
		return nil
	}
	clone := *route
	clone.Tags = slices.Clone(route.Tags)
	clone.Security = cloneSecurity(route.Security)
	clone.middleware = slices.Clone(route.middleware)
	if route.Schema != nil {
		schema := *route.Schema
		schema.Parameters = slices.Clone(route.Schema.Parameters)
		schema.Auth = slices.Clone(route.Schema.Auth)
		clone.Schema = &schema
	}
	return &clone
}
