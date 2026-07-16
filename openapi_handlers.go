package aku

import (
	"html/template"
	"net/http"
	"sync"

	"github.com/nijaru/aku/openapi"
)

// OpenAPIDocument generates an OpenAPI 3.0 document for the application.
func (a *App) OpenAPIDocument(title, version string) *openapi.Document {
	a.mu.RLock()
	iroutes := make([]openapi.Route, len(a.routes))
	for i, r := range a.routes {
		iroutes[i] = cloneRoute(r)
	}

	schemes := make(map[string]openapi.SecurityScheme, len(a.securitySchemes))
	for name, s := range a.securitySchemes {
		schemes[name] = openapi.SecurityScheme{
			Type:             s.Type,
			Description:      s.Description,
			Name:             s.Name,
			In:               s.In,
			Scheme:           s.Scheme,
			BearerFormat:     s.BearerFormat,
			OpenIdConnectUrl: s.OpenIdConnectUrl,
		}
	}
	a.mu.RUnlock()

	return openapi.Generate(title, version, iroutes, schemes)
}

// OpenAPI registers an endpoint that serves the OpenAPI JSON specification.
func (a *App) OpenAPI(pattern, title, version string) {
	a.registerHandler("GET "+pattern, a.OpenAPIHandler(title, version))
}

// SwaggerUI registers an endpoint that serves the Swagger UI.
func (a *App) SwaggerUI(pattern, specURL string) {
	a.registerHandler("GET "+pattern, a.SwaggerUIHandler(specURL))
}

// RedocUI registers an endpoint that serves the Redoc UI.
func (a *App) RedocUI(pattern, specURL string) {
	a.registerHandler("GET "+pattern, a.RedocUIHandler(specURL))
}

// OpenAPIHandler returns an http.Handler that serves the OpenAPI JSON specification.
func (a *App) OpenAPIHandler(title, version string) http.Handler {
	var cacheMu sync.RWMutex
	var (
		cache        []byte
		cacheErr     error
		cacheVersion uint64
		cacheValid   bool
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		versionNumber := a.openapiVersion
		a.mu.RUnlock()

		cacheMu.RLock()
		if cacheValid && cacheVersion == versionNumber {
			cached, cachedErr := cache, cacheErr
			cacheMu.RUnlock()
			writeOpenAPICache(w, cached, cachedErr)
			return
		}
		cacheMu.RUnlock()

		doc := a.OpenAPIDocument(title, version)
		generated, generatedErr := doc.JSON()

		a.mu.RLock()
		currentVersion := a.openapiVersion
		a.mu.RUnlock()
		if currentVersion == versionNumber {
			cacheMu.Lock()
			cache = generated
			cacheErr = generatedErr
			cacheVersion = versionNumber
			cacheValid = true
			cacheMu.Unlock()
		}

		writeOpenAPICache(w, generated, generatedErr)
	})
}

func writeOpenAPICache(w http.ResponseWriter, cache []byte, cacheErr error) {
	if cacheErr != nil {
		http.Error(w, "Failed to generate OpenAPI spec", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cache)
}

// SwaggerUIHandler returns an http.Handler that serves the Swagger UI.
// The specURL is the URL where the OpenAPI JSON is served (e.g., "/openapi.json").
func (a *App) SwaggerUIHandler(specURL string) http.Handler {
	escapedSpecURL := template.JSEscapeString(specURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.32.6/swagger-ui.css" >
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.32.6/swagger-ui-bundle.js"> </script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.32.6/swagger-ui-standalone-preset.js"> </script>
    <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "` + escapedSpecURL + `",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout"
      });
      window.ui = ui;
    };
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})
}

// RedocUIHandler returns an http.Handler that serves the Redoc UI.
// The specURL is the URL where the OpenAPI JSON is served (e.g., "/openapi.json").
func (a *App) RedocUIHandler(specURL string) http.Handler {
	escapedSpecURL := template.HTMLEscapeString(specURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Redoc</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
    <redoc spec-url="` + escapedSpecURL + `"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"> </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})
}
