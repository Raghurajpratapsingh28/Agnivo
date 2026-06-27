package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/validation"
)

// MaxBodyBytes bounds request bodies to prevent memory exhaustion (1 MB).
const MaxBodyBytes = 1 << 20

// Decode reads and JSON-decodes the request body into dst, rejecting unknown
// fields and oversized bodies. It does not validate; call DecodeAndValidate for
// that.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return ErrBadRequest("request body too large")
		case errors.Is(err, io.EOF):
			return ErrBadRequest("request body is empty")
		default:
			return ErrBadRequest("malformed JSON body")
		}
	}
	if dec.More() {
		return ErrBadRequest("request body must contain a single JSON object")
	}
	return nil
}

// DecodeAndValidate decodes the body and validates it with the shared validator,
// returning a 422 with field details on validation failure.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := Decode(w, r, dst); err != nil {
		return err
	}
	if err := validation.Default().Struct(dst); err != nil {
		var verr *validation.Error
		if errors.As(err, &verr) {
			return ErrUnprocessable("validation failed").WithDetails(verr.Fields)
		}
		return ErrBadRequest(err.Error())
	}
	return nil
}
