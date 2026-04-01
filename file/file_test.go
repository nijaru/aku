package file

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/textproto"
	"testing"
)

func createTestFile(t *testing.T, filename, contentType string, data []byte) *multipart.FileHeader {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	fw, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = w.Close()

	r := multipart.NewReader(&b, w.Boundary())
	form, err := r.ReadForm(1024)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })

	// The created file should be in the form
	for _, files := range form.File {
		if len(files) > 0 {
			return files[0]
		}
	}
	t.Fatal("no files found in form")
	return nil
}

func TestFile_Open(t *testing.T) {
	fh := createTestFile(t, "test.txt", "text/plain", []byte("hello world"))

	f := File{Header: fh}
	rc, size, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	if size != 11 {
		t.Fatalf("expected size 11, got %d", size)
	}

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected content: %s", string(data))
	}
}

func TestFile_Open_NilHeader(t *testing.T) {
	f := File{Header: nil}
	_, _, err := f.Open()
	if err == nil {
		t.Fatal("expected error for nil header")
	}
}

func TestFile_ContentType(t *testing.T) {
	fh := createTestFile(t, "test.txt", "text/plain", []byte("hello world"))
	f := File{Header: fh}

	ct, err := f.ContentType()
	if err != nil {
		t.Fatal(err)
	}
	if ct != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", ct)
	}
}

func TestFile_ContentType_ImagePNG(t *testing.T) {
	// PNG needs 512+ bytes for http.DetectContentType
	sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	data := append(sig, bytes.Repeat([]byte{0x00}, 512)...)
	fh := createTestFile(t, "photo.png", "application/octet-stream", data)
	f := File{Header: fh}

	ct, err := f.ContentType()
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" {
		t.Fatalf("expected image/png, got: %q", ct)
	}
}

func TestFile_ValidateSize(t *testing.T) {
	fh := createTestFile(t, "small.txt", "text/plain", []byte("hi"))
	f := File{Header: fh}

	if err := f.ValidateSize(100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := f.ValidateSize(1); err == nil {
		t.Fatal("expected error for oversized file")
	}
}

func TestFile_ValidateMIME(t *testing.T) {
	fh := createTestFile(t, "test.txt", "text/plain", []byte("hello world"))
	f := File{Header: fh}

	if err := f.ValidateMIME("text/plain; charset=utf-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := f.ValidateMIME("application/json"); err == nil {
		t.Fatal("expected MIME validation error")
	}
}

func TestFile_Validate(t *testing.T) {
	fh := createTestFile(t, "small.txt", "text/plain", []byte("hello"))
	f := File{Header: fh}

	// Both size and MIME valid
	if err := f.Validate(100, "text/plain; charset=utf-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Size too big
	if err := f.Validate(1, "text/plain; charset=utf-8"); err == nil {
		t.Fatal("expected size error")
	}
}

func TestFiles_ValidateAll(t *testing.T) {
	fh1 := createTestFile(t, "one.txt", "text/plain", []byte("first"))
	fh2 := createTestFile(t, "two.txt", "text/plain", []byte("second"))

	files := Files{Headers: []*multipart.FileHeader{fh1, fh2}}

	if err := files.ValidateAll(100, "text/plain; charset=utf-8"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := files.ValidateAll(2); err == nil {
		t.Fatal("expected error for oversized files")
	}
}
