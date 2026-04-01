package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nijaru/aku"
)

func StreamReader(ctx context.Context, _ struct{}) (aku.Stream, error) {
	reader := strings.NewReader("This is a streamed response from an io.Reader")
	return aku.Stream{
		Reader:      reader,
		ContentType: "text/plain",
	}, nil
}

func StreamSSE(ctx context.Context, _ struct{}) (*aku.SSE, error) {
	events := make(chan aku.Event)

	go func() {
		defer close(events)
		for i := range 5 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				events <- aku.Event{
					ID:   fmt.Sprintf("%d", i),
					Data: fmt.Sprintf("Event number %d at %s", i, time.Now().Format(time.TimeOnly)),
				}
			}
		}
	}()

	return &aku.SSE{Events: events}, nil
}

func main() {
	app := aku.New()

	aku.Get(app, "/stream", StreamReader)
	aku.Get(app, "/events", StreamSSE)

	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("Try: curl -N http://localhost:8080/events")
	log.Fatal(http.ListenAndServe(":8080", app))
}
