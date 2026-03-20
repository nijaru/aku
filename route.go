package aku

import (
	"net/http"
	"reflect"

	"github.com/nijaru/aku/internal/bind"
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
	Schema      *bind.Schema
	OutputType  reflect.Type
	middleware  []func(http.Handler) http.Handler
}

func (r *Route) GetMethod() string                  { return r.Method }
func (r *Route) GetPattern() string                 { return r.Pattern }
func (r *Route) GetStatus() int                     { return r.Status }
func (r *Route) GetSummary() string                 { return r.Summary }
func (r *Route) GetDescription() string             { return r.Description }
func (r *Route) GetOperationID() string             { return r.OperationID }
func (r *Route) GetDeprecated() bool                { return r.Deprecated }
func (r *Route) GetInternal() bool                  { return r.Internal }
func (r *Route) GetTags() []string                  { return r.Tags }
func (r *Route) GetSecurity() []map[string][]string { return r.Security }
func (r *Route) GetSchema() *bind.Schema            { return r.Schema }
func (r *Route) GetOutputType() reflect.Type        { return r.OutputType }
