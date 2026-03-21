package aku

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/problem"
)

// WebsocketHandler is a typesafe websocket handler.
type WebsocketHandler[In any, Msg any] func(ctx context.Context, in In, ws *Websocket[Msg]) error

// Websocket is a typesafe wrapper around a websocket connection.
type Websocket[Msg any] struct {
	conn *websocket.Conn
}

// Receive receives a JSON message from the websocket connection.
func (ws *Websocket[Msg]) Receive(ctx context.Context) (Msg, error) {
	var msg Msg
	err := wsjson.Read(ctx, ws.conn, &msg)
	return msg, err
}

// Send sends a JSON message to the websocket connection.
func (ws *Websocket[Msg]) Send(ctx context.Context, msg Msg) error {
	return wsjson.Write(ctx, ws.conn, msg)
}

// Close closes the websocket connection with the given code and reason.
func (ws *Websocket[Msg]) Close(code websocket.StatusCode, reason string) error {
	return ws.conn.Close(code, reason)
}

// WS registers a typesafe websocket route on the application or group.
func WS[In any, Msg any](r Router, pattern string, handler WebsocketHandler[In, Msg], opts ...RouteOption) error {
	app := r.App()
	meta := defaultRouteMeta()

	// Compile the extractor and schema once at startup.
	extractor, schema := bind.Compiler[In]()
	meta.schema = schema

	// Extract custom messages from the input struct (Query, Path, etc.)
	customMessages := bind.GetCustomMessages(reflect.TypeOf((*In)(nil)).Elem())

	for _, opt := range opts {
		opt(&meta)
	}

	// Pool for input structs to minimize allocations.
	type PooledIn struct {
		ptr *In
		val reflect.Value
	}
	pool := sync.Pool{
		New: func() any {
			ptr := new(In)
			return &PooledIn{
				ptr: ptr,
				val: reflect.ValueOf(ptr).Elem(),
			}
		},
	}

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pooled := pool.Get().(*PooledIn)
		in := pooled.ptr
		defer func() {
			var zero In
			*in = zero
			pool.Put(pooled)
		}()

		// 1. Extract and bind parameters from the handshake request.
		if err := extractor(r.Context(), r, in, pooled.val, app.bindConfig); err != nil {
			if bindErr, ok := errors.AsType[*bind.BindError](err); ok {
				handleError(app, w, r, problem.ValidationProblem("Handshake extraction failed", []problem.InvalidParam{
					{Name: bindErr.Field, In: bindErr.Source, Reason: bindErr.Err.Error()},
				}))
			} else {
				handleError(app, w, r, err)
			}
			return
		}

		// 2. Run validator if present.
		if app.validator != nil {
			if err := app.validator.Struct(in); err != nil {
				if vErr, ok := errors.AsType[validator.ValidationErrors](err); ok {
					handleError(app, w, r, problem.ValidationProblem("Handshake validation failed", problem.FromValidationErrors(vErr, customMessages)))
				} else {
					handleError(app, w, r, problem.BadRequest(err.Error()))
				}
				return
			}
		}

		// 3. Upgrade the connection.
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			// websocket.Accept handles the error response if it fails.
			return
		}
		defer c.Close(websocket.StatusInternalError, "unexpected disconnection")

		ws := &Websocket[Msg]{conn: c}

		// 4. Call the user handler.
		if err := handler(r.Context(), *in, ws); err != nil {
			handleError(app, w, r, err)
		}
	})

	// Apply middleware (outermost first).
	var finalHandler http.Handler = h
	for i := len(meta.middleware) - 1; i >= 0; i-- {
		finalHandler = meta.middleware[i](finalHandler)
	}
	groupMW := r.Middleware()
	for i := len(groupMW) - 1; i >= 0; i-- {
		finalHandler = groupMW[i](finalHandler)
	}

	fullPattern := r.Prefix() + pattern
	route := &Route{
		Method:      "GET", // Handshake is always GET
		Pattern:     fullPattern,
		Status:      101, // Switching Protocols
		Summary:     meta.summary,
		Description: meta.description,
		OperationID: meta.operationID,
		Deprecated:  meta.deprecated,
		Internal:    meta.internal,
		Tags:        meta.tags,
		Security:    meta.security,
		Schema:      schema,
		middleware:  append(append([]func(http.Handler) http.Handler{}, groupMW...), meta.middleware...),
	}

	r.Handle("GET", pattern, finalHandler, route)
	return nil
}
