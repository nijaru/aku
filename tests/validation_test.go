package aku_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/problem"
)

type ValidatedRequest struct {
	Body struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"min=18"`
	}
}

type ValidatedResponse struct {
	Message string `json:"message"`
}

func TestValidation(t *testing.T) {
	app := aku.New(aku.WithValidator(validator.New()))

	aku.Post(app, "/test", func(ctx context.Context, in ValidatedRequest) (ValidatedResponse, error) {
		return ValidatedResponse{Message: "Success"}, nil
	})

	t.Run("Valid request", func(t *testing.T) {
		body := `{"email": "test@example.com", "age": 20}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Invalid email", func(t *testing.T) {
		body := `{"email": "invalid-email", "age": 20}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("Expected status 422, got %d", w.Code)
		}

		var prob problem.Details
		if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
			t.Fatal(err)
		}

		found := false
		for _, p := range prob.InvalidParams {
			if p.Name == "Email" && p.Reason == "must be a valid email address" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected invalid email parameter in problem, got %+v", prob.InvalidParams)
		}
	})

	t.Run("Under age", func(t *testing.T) {
		body := `{"email": "test@example.com", "age": 10}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("Expected status 422, got %d", w.Code)
		}

		var prob problem.Details
		if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
			t.Fatal(err)
		}

		found := false
		for _, p := range prob.InvalidParams {
			if p.Name == "Age" && p.Reason == "must be at least 18" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected invalid age parameter in problem, got %+v", prob.InvalidParams)
		}
	})
}

func TestGlobalErrorHandler(t *testing.T) {
	customHandlerCalled := false
	app := aku.New(aku.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
		customHandlerCalled = true
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("Custom Error"))
	}))

	aku.Post(app, "/error", func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, problem.BadRequest("Something went wrong")
	})

	req := httptest.NewRequest(http.MethodPost, "/error", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if !customHandlerCalled {
		t.Error("Custom error handler was not called")
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("Expected status 418, got %d", w.Code)
	}
	if w.Body.String() != "Custom Error" {
		t.Errorf("Expected body 'Custom Error', got %q", w.Body.String())
	}
}

type FileUploadRequest struct {
	Form struct {
		Name string                `form:"name"`
		File *multipart.FileHeader `form:"file"`
	}
}

func TestFileUpload(t *testing.T) {
	app := aku.New()

	aku.Post(app, "/upload", func(ctx context.Context, in FileUploadRequest) (ValidatedResponse, error) {
		if in.Form.Name != "test-file" {
			return ValidatedResponse{}, problem.BadRequest("Invalid name")
		}
		if in.Form.File == nil {
			return ValidatedResponse{}, problem.BadRequest("File is missing")
		}
		if in.Form.File.Filename != "hello.txt" {
			return ValidatedResponse{}, problem.BadRequest("Invalid filename")
		}
		return ValidatedResponse{Message: "Uploaded " + in.Form.File.Filename}, nil
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("hello world"))
	writer.WriteField("name", "test-file")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ValidatedResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Message != "Uploaded hello.txt" {
		t.Errorf("Expected message 'Uploaded hello.txt', got %q", resp.Message)
	}
}
