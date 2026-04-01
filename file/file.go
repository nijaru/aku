package file

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
)

// File wraps a multipart file header with validation helpers.
// Extract via the Form section of your input struct:
//
//	type In struct {
//		Photo *multipart.FileHeader `form:"photo"`
//	}
//
// Then wrap: f := file.File{Header: in.Photo}
type File struct {
	Header *multipart.FileHeader
}

// Open opens the file for reading. Returns the file size along with the reader.
func (f File) Open() (fh io.ReadCloser, size int64, err error) {
	if f.Header == nil {
		return nil, 0, fmt.Errorf("file header is nil")
	}
	rc, err := f.Header.Open()
	if err != nil {
		return nil, 0, err
	}
	return rc, f.Header.Size, nil
}

// ContentType sniffs the content type from the file header.
// Returns the MIME type (e.g., "image/png", "application/pdf").
func (f File) ContentType() (string, error) {
	if f.Header == nil {
		return "", fmt.Errorf("file header is nil")
	}
	rc, err := f.Header.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(rc, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// ValidateSize returns an error if the file exceeds maxBytes.
func (f File) ValidateSize(maxBytes int64) error {
	if f.Header == nil {
		return fmt.Errorf("file header is nil")
	}
	if f.Header.Size > maxBytes {
		return fmt.Errorf("file size %d exceeds maximum %d bytes", f.Header.Size, maxBytes)
	}
	return nil
}

// ValidateMIME returns an error if the detected content type is not in the allowed list.
func (f File) ValidateMIME(allowed ...string) error {
	if f.Header == nil {
		return fmt.Errorf("file header is nil")
	}
	ct, err := f.ContentType()
	if err != nil {
		return fmt.Errorf("failed to detect content type: %w", err)
	}
	if !slices.Contains(allowed, ct) {
		return fmt.Errorf("file content type %q is not allowed", ct)
	}
	return nil
}

// Validate validates size and MIME type in one call.
func (f File) Validate(maxBytes int64, allowedMIME ...string) error {
	if err := f.ValidateSize(maxBytes); err != nil {
		return err
	}
	if len(allowedMIME) > 0 {
		return f.ValidateMIME(allowedMIME...)
	}
	return nil
}

// Files is a convenience type for multiple file uploads.
type Files struct {
	Headers []*multipart.FileHeader
}

// ValidateAll validates that all files meet the size and MIME constraints.
// Returns the first validation error encountered.
func (f Files) ValidateAll(maxBytes int64, allowedMIME ...string) error {
	for _, h := range f.Headers {
		wrap := File{Header: h}
		if err := wrap.Validate(maxBytes, allowedMIME...); err != nil {
			return err
		}
	}
	return nil
}
