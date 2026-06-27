package httpx

import (
	"io"
	"mime/multipart"
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

// DefaultMultipartMaxMemory is the default memory threshold for multipart
// parsing (32 MiB). Parts larger than this spill to disk.
const DefaultMultipartMaxMemory = 32 << 20

// ParseMultipart parses a multipart/form-data request. maxMemory bounds the
// amount stored in memory before spilling to disk; zero uses the default.
func ParseMultipart(r *http.Request, maxMemory int64) (*multipart.Form, error) {
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMaxMemory
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "invalid multipart form")
	}
	return r.MultipartForm, nil
}

// FormValue returns the first value for key from a parsed multipart form or URL
// form, or "" when absent.
func FormValue(r *http.Request, key string) string {
	if r.MultipartForm != nil && r.MultipartForm.Value != nil {
		if vals := r.MultipartForm.Value[key]; len(vals) > 0 {
			return vals[0]
		}
	}
	return r.FormValue(key)
}

// FormFile opens the first uploaded file for key. The caller must close the
// returned ReadCloser. Returns CodeInvalidArgument when the part is missing.
func FormFile(r *http.Request, key string) (multipart.File, *multipart.FileHeader, error) {
	f, hdr, err := r.FormFile(key)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.CodeInvalidArgument, "missing or invalid form file")
	}
	return f, hdr, nil
}

// ReadFormFile reads the entire contents of the first uploaded file for key,
// enforcing a maxBytes limit to guard against oversized uploads.
func ReadFormFile(r *http.Request, key string, maxBytes int64) ([]byte, *multipart.FileHeader, error) {
	f, hdr, err := FormFile(r, key)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, hdr, errors.Wrap(err, errors.CodeInvalidArgument, "read form file")
	}
	if int64(len(data)) > maxBytes {
		return nil, hdr, errors.New(errors.CodeInvalidArgument, "uploaded file too large")
	}
	return data, hdr, nil
}
