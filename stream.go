package aku

import "io"

// Event represents a single Server-Sent Event.
type Event struct {
	ID    string
	Event string
	Data  any
}

// SSE represents a Server-Sent Events stream.
type SSE struct {
	Events <-chan Event
}

// Stream represents a generic data stream with an optional content type.
type Stream struct {
	Reader      io.Reader
	ContentType string
}
