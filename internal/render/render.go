package render

import (
	"encoding/json"
	"net/http"
)

// JSON renders a success payload as JSON with the specified status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Problem renders an RFC 9457 Problem Details for HTTP APIs.
func Problem(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
