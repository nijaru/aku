package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nijaru/aku"
)

// Tester provides a fluent API for testing Aku applications.
type Tester struct {
	t   testing.TB
	app *aku.App
}

// Test creates a new Tester for the given app.
func Test(t testing.TB, app *aku.App) *Tester {
	return &Tester{t: t, app: app}
}

// RequestBuilder builds an HTTP request for testing.
type RequestBuilder struct {
	tester *Tester
	method string
	path   string
	header http.Header
	body   []byte
}

// Get starts a GET request.
func (t *Tester) Get(path string) *RequestBuilder {
	return &RequestBuilder{tester: t, method: http.MethodGet, path: path, header: make(http.Header)}
}

// Post starts a POST request.
func (t *Tester) Post(path string) *RequestBuilder {
	return &RequestBuilder{tester: t, method: http.MethodPost, path: path, header: make(http.Header)}
}

// Put starts a PUT request.
func (t *Tester) Put(path string) *RequestBuilder {
	return &RequestBuilder{tester: t, method: http.MethodPut, path: path, header: make(http.Header)}
}

// Patch starts a PATCH request.
func (t *Tester) Patch(path string) *RequestBuilder {
	return &RequestBuilder{tester: t, method: http.MethodPatch, path: path, header: make(http.Header)}
}

// Delete starts a DELETE request.
func (t *Tester) Delete(path string) *RequestBuilder {
	return &RequestBuilder{tester: t, method: http.MethodDelete, path: path, header: make(http.Header)}
}

// WithHeader adds a header to the request.
func (r *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	r.header.Add(key, value)
	return r
}

// WithJSON sets the request body to the JSON representation of v.
func (r *RequestBuilder) WithJSON(v any) *RequestBuilder {
	r.tester.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		r.tester.t.Fatalf("failed to marshal JSON body: %v", err)
	}
	r.body = data
	r.header.Set("Content-Type", "application/json")
	return r
}

// WithBody sets the request body.
func (r *RequestBuilder) WithBody(body io.Reader) *RequestBuilder {
	data, err := io.ReadAll(body)
	if err != nil {
		r.tester.t.Fatalf("failed to read request body: %v", err)
	}
	r.body = data
	return r
}

// Response represents the response from a tested request.
type Response struct {
	tester *Tester
	resp   *httptest.ResponseRecorder
}

// Do executes the request and returns the response.
func (r *RequestBuilder) Do() *Response {
	r.tester.t.Helper()
	var bodyReader io.Reader
	if r.body != nil {
		bodyReader = bytes.NewReader(r.body)
	}
	req := httptest.NewRequest(r.method, r.path, bodyReader)
	for k, v := range r.header {
		req.Header[k] = v
	}
	w := httptest.NewRecorder()
	r.tester.app.ServeHTTP(w, req)
	return &Response{tester: r.tester, resp: w}
}

// ExpectStatus asserts that the response status code matches expected.
// If Do() hasn't been called yet, it will be called automatically.
func (r *RequestBuilder) ExpectStatus(expected int) *Response {
	r.tester.t.Helper()
	return r.Do().ExpectStatus(expected)
}

// ExpectStatus asserts that the response status code matches expected.
func (r *Response) ExpectStatus(expected int) *Response {
	r.tester.t.Helper()
	if r.resp.Code != expected {
		r.tester.t.Errorf("expected status %d %s, got %d %s",
			expected, http.StatusText(expected),
			r.resp.Code, http.StatusText(r.resp.Code))
	}
	return r
}

// ExpectJSON asserts that the response body matches the JSON representation of expected.
func (r *Response) ExpectJSON(expected any) *Response {
	r.tester.t.Helper()

	if expected == nil {
		if r.resp.Body.Len() > 0 {
			r.tester.t.Errorf("expected empty body, got %q", r.resp.Body.String())
		}
		return r
	}

	// Unmarshal both into a map or slice for normalized comparison (handles float64 issues)
	var actualVal any
	if err := json.Unmarshal(r.resp.Body.Bytes(), &actualVal); err != nil {
		r.tester.t.Fatalf("failed to unmarshal response body: %v\nBody: %s", err, r.resp.Body.String())
	}

	expectedJSON, _ := json.Marshal(expected)
	var expectedVal any
	json.Unmarshal(expectedJSON, &expectedVal)

	if !reflect.DeepEqual(actualVal, expectedVal) {
		r.tester.t.Errorf("expected JSON response %+v, got %+v", expected, actualVal)
	}

	return r
}

// ExpectHeader asserts that the response header matches expected.
func (r *Response) ExpectHeader(key, expected string) *Response {
	r.tester.t.Helper()
	actual := r.resp.Header().Get(key)
	if actual != expected {
		r.tester.t.Errorf("expected header %q to be %q, got %q", key, expected, actual)
	}
	return r
}

// ExpectBody asserts that the response body matches expected string.
func (r *Response) ExpectBody(expected string) *Response {
	r.tester.t.Helper()
	actual := r.resp.Body.String()
	if actual != expected {
		r.tester.t.Errorf("expected body %q, got %q", expected, actual)
	}
	return r
}

// Body returns the response body as a byte slice.
func (r *Response) Body() []byte {
	return r.resp.Body.Bytes()
}

// Header returns the response headers.
func (r *Response) Header() http.Header {
	return r.resp.Header()
}

// Recorder returns the underlying httptest.ResponseRecorder.
func (r *Response) Recorder() *httptest.ResponseRecorder {
	return r.resp
}
