package render

import (
	"encoding/json"
	"fmt"
	"io"
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

// Reader writes the reader content to the response with the given status and content type.
func Reader(w http.ResponseWriter, status int, r io.Reader, contentType string) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, r)
	if rc, ok := r.(io.ReadCloser); ok {
		_ = rc.Close()
	}
}

// SSEEvent represents a single Server-Sent Event for the renderer.
type SSEEvent struct {
	ID    string
	Event string
	Data  any
}

// SSE streams Server-Sent Events from a channel.
func SSE(w http.ResponseWriter, r *http.Request, events <-chan SSEEvent) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Note: Transfer-Encoding chunked is automatically added by Go if we don't set Content-Length.

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.ID != "" {
				fmt.Fprintf(w, "id: %s\n", event.ID)
			}
			if event.Event != "" {
				fmt.Fprintf(w, "event: %s\n", event.Event)
			}
			if event.Data != nil {
				switch d := event.Data.(type) {
				case string:
					fmt.Fprintf(w, "data: %s\n", d)
				case []byte:
					fmt.Fprintf(w, "data: %s\n", string(d))
				default:
					b, _ := json.Marshal(d)
					fmt.Fprintf(w, "data: %s\n", string(b))
				}
			}
			fmt.Fprintf(w, "\n")
			flusher.Flush()
		}
	}
}
