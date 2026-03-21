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

	aku.WS(app, "/chat", func(ctx context.Context, in WSHandshake, ws *aku.Websocket[WSMessage]) error {
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
	})

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
