package httpx

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// Attachment streams content to the client as a file download with
// Content-Disposition: attachment. contentType defaults to
// application/octet-stream when empty.
func Attachment(w http.ResponseWriter, filename, contentType string, r io.Reader) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(filename)))
	w.WriteHeader(http.StatusOK)
	_, err := io.Copy(w, r)
	return err
}

// AttachmentBytes is Attachment for an in-memory payload.
func AttachmentBytes(w http.ResponseWriter, filename, contentType string, data []byte) error {
	w.Header().Set("Content-Type", contentTypeOr(contentType))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(filename)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(data)
	return err
}

// Inline streams content with Content-Disposition: inline for browser rendering.
func Inline(w http.ResponseWriter, filename, contentType string, r io.Reader) error {
	w.Header().Set("Content-Type", contentTypeOr(contentType))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, sanitizeFilename(filename)))
	w.WriteHeader(http.StatusOK)
	_, err := io.Copy(w, r)
	return err
}

func contentTypeOr(ct string) string {
	if ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// sanitizeFilename strips path components and quotes from a filename so it is
// safe to embed in a Content-Disposition header.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	if name == "" || name == "." {
		return "download"
	}
	return name
}
