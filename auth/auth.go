// Package auth provides types and middleware for authenticated endpoints in Aku.
//
// Use with the Auth section of Aku handler inputs:
//
//	type Input struct {
//	    Auth struct {
//	        Token auth.Bearer `msg:"Missing or invalid token"`
//	    }
//	    Path struct {
//	        ID string `path:"id"`
//	    }
//	}
//
//	func GetUser(ctx context.Context, in Input) (User, error) {
//	    token := in.Auth.Token // string, the bearer token
//	    ...
//	}
package auth

import (
	"net/http"
)

// Bearer is a simple string alias representing a bearer token.
// Place it in the Auth section of a handler input struct.
type Bearer string

// APIKey represents an API key credential.
// Place it in the Auth section with an `auth:"apikey:header:X-API-Key"` tag.
type APIKey string

// RequireBearer middleware returns 401 if the Authorization header does not
// contain a valid Bearer token. Intended for group-level or global middleware
// when you don't need the token value in the handler.
func RequireBearer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if len(auth) < 7 || auth[:7] != "Bearer " || auth[7:] == "" {
				write401(w, "Missing or invalid bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKey middleware returns 401 if the specified header is missing
// or empty. Use for API-key-protected routes when the key value isn't needed
// in the handler itself.
func RequireAPIKey(headerName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(headerName) == "" {
				write401(w, "Missing API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func write401(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(
		`{"type":"about:blank","title":"Unauthorized","status":401,"detail":"` + detail + `"}`,
	))
}
