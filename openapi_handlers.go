package aku

import (
	"net/http"
	"sync"

	"github.com/nijaru/aku/internal/openapi"
)

// OpenAPIDocument generates an OpenAPI 3.0 document for the application.
func (a *App) OpenAPIDocument(title, version string) *openapi.Document {
	iroutes := make([]openapi.Route, len(a.routes))
	for i, r := range a.routes {
		iroutes[i] = r
	}

	schemes := make(map[string]openapi.SecurityScheme)
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

	return openapi.Generate(title, version, iroutes, schemes)
}

// OpenAPI registers an endpoint that serves the OpenAPI JSON specification.
func (a *App) OpenAPI(pattern, title, version string) {
	a.mux.Handle("GET "+pattern, a.OpenAPIHandler(title, version))
}

// SwaggerUI registers an endpoint that serves the Swagger UI.
func (a *App) SwaggerUI(pattern, specURL string) {
	a.mux.Handle("GET "+pattern, a.SwaggerUIHandler(specURL))
}

// RedocUI registers an endpoint that serves the Redoc UI.
func (a *App) RedocUI(pattern, specURL string) {
	a.mux.Handle("GET "+pattern, a.RedocUIHandler(specURL))
}

// OpenAPIHandler returns an http.Handler that serves the OpenAPI JSON specification.
func (a *App) OpenAPIHandler(title, version string) http.Handler {
	var (
		cache     []byte
		cacheOnce sync.Once
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		cacheOnce.Do(func() {
			doc := a.OpenAPIDocument(title, version)
			cache, err = doc.JSON()
		})

		if err != nil {
			http.Error(w, "Failed to generate OpenAPI spec", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(cache)
	})
}

// SwaggerUIHandler returns an http.Handler that serves the Swagger UI.
// The specURL is the URL where the OpenAPI JSON is served (e.g., "/openapi.json").
func (a *App) SwaggerUIHandler(specURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui.css" >
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-bundle.js"> </script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-standalone-preset.js"> </script>
    <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "` + specURL + `",
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
    <redoc spec-url="` + specURL + `"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"> </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})
}
