package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
	"github.com/nijaru/aku/middleware"
)

type WSHandshake struct {
	Query struct {
		Token string `query:"token" validate:"required"`
	}
}

type WSMessage struct {
	Text string `json:"text"`
}

func TestWebsocket(t *testing.T) {
	app := aku.New()
	app.Use(middleware.Logger)

	aku.WS(
		app,
		"/chat",
		func(ctx context.Context, in WSHandshake, ws *aku.Websocket[WSMessage]) error {
			if in.Query.Token != "secret" {
				return nil // Handled by Accept but we test extraction here
			}

			for {
				msg, err := ws.Receive(ctx)
				if err != nil {
					return err
				}
				if err := ws.Send(ctx, WSMessage{Text: "echo: " + msg.Text}); err != nil {
					return err
				}
			}
		},
	)

	srv := httptest.NewServer(app)
	defer srv.Close()

	t.Run("Rejects invalid handshake", func(t *testing.T) {
		url := strings.Replace(srv.URL, "http", "ws", 1) + "/chat"
		_, resp, err := websocket.Dial(context.Background(), url, nil)
		if err == nil {
			t.Logf("Response: %+v", resp)
			t.Fatal("expected error for missing token")
		}
		if resp == nil {
			t.Fatalf("expected response for failed handshake, got nil: %v", err)
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", resp.StatusCode)
		}
	})

	t.Run("Success handshake and echo", func(t *testing.T) {
		url := strings.Replace(srv.URL, "http", "ws", 1) + "/chat?token=secret"
		ctx := context.Background()
		c, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")

		msg := WSMessage{Text: "hello"}
		if err := wsjson.Write(ctx, c, msg); err != nil {
			t.Fatalf("failed to write: %v", err)
		}

		var echo WSMessage
		if err := wsjson.Read(ctx, c, &echo); err != nil {
			t.Fatalf("failed to read: %v", err)
		}

		if echo.Text != "echo: hello" {
			t.Errorf("expected 'echo: hello', got '%s'", echo.Text)
		}
	})
}

func TestWebsocketAuthAppearsInOpenAPI(t *testing.T) {
	app := aku.New()
	type In struct {
		Auth struct {
			Token auth.Bearer
			Key   auth.APIKey `auth:"apikey:header:X-API-Key"`
		}
	}

	if err := aku.WS(app, "/chat", func(ctx context.Context, in In, ws *aku.Websocket[WSMessage]) error {
		return nil
	}); err != nil {
		t.Fatalf("unexpected registration error: %v", err)
	}

	doc := app.OpenAPIDocument("Chat API", "1.0.0")
	scheme, ok := doc.Components.SecuritySchemes["Token"]
	if !ok {
		t.Fatal("expected websocket bearer scheme in OpenAPI components")
	}
	if scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Fatalf("unexpected websocket security scheme: %+v", scheme)
	}
	security := doc.Paths["/chat"]["get"].Security
	if len(security) != 1 {
		t.Fatalf("expected websocket security requirement, got %+v", security)
	}
	if _, ok := security[0]["Token"]; !ok {
		t.Fatalf("expected Token security requirement, got %+v", security)
	}
	if _, ok := security[0]["Key"]; !ok {
		t.Fatalf("expected Key security requirement, got %+v", security)
	}
	if security[0]["Token"] == nil || security[0]["Key"] == nil {
		t.Fatalf("security scopes must be JSON arrays, got %+v", security[0])
	}
}

func TestWebsocketAuthFailureIsUnauthorized(t *testing.T) {
	app := aku.New()
	type In struct {
		Auth struct {
			Token auth.Bearer
		}
	}

	if err := aku.WS(app, "/chat", func(ctx context.Context, in In, ws *aku.Websocket[WSMessage]) error {
		return nil
	}); err != nil {
		t.Fatalf("unexpected registration error: %v", err)
	}

	srv := httptest.NewServer(app)
	defer srv.Close()

	url := strings.Replace(srv.URL, "http", "ws", 1) + "/chat"
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatal("expected websocket handshake to fail")
	}
	if resp == nil {
		t.Fatalf("expected failed handshake response, got nil: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("expected Bearer challenge, got %q", got)
	}
}

func TestWebsocketRejectsMismatchedPathBinding(t *testing.T) {
	app := aku.New()
	type In struct {
		Path struct {
			ID string `path:"other"`
		}
	}

	err := aku.WS(
		app,
		"/chat/{id}",
		func(ctx context.Context, in In, ws *aku.Websocket[WSMessage]) error {
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no matching path binding") {
		t.Fatalf("expected websocket path binding error, got %v", err)
	}
}
