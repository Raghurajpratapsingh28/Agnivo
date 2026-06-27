package dto

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/validation"
)

// MaxBodyBytes bounds request bodies to prevent memory exhaustion (1 MiB).
const MaxBodyBytes = 1 << 20

// Decode reads and JSON-decodes the request body into dst, rejecting unknown
// fields, oversized bodies, and trailing content. It returns typed platform
// errors (CodeInvalidArgument) so the transport maps them consistently.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case stderrors.As(err, &maxErr):
			return errors.New(errors.CodeInvalidArgument, "request body too large")
		case stderrors.Is(err, io.EOF):
			return errors.New(errors.CodeInvalidArgument, "request body is empty")
		default:
			return errors.Wrap(err, errors.CodeInvalidArgument, "malformed JSON body")
		}
	}
	if dec.More() {
		return errors.New(errors.CodeInvalidArgument, "request body must contain a single JSON object")
	}
	return nil
}

// DecodeValidate decodes the body and validates it with the shared validator,
// returning a CodeValidation error carrying field details on failure.
func DecodeValidate(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := Decode(w, r, dst); err != nil {
		return err
	}
	return Validate(dst)
}

// Validate runs struct validation on an already-populated value, returning a
// CodeValidation error whose cause is the underlying validation.Error so the
// field details surface in the response.
func Validate(dst any) error {
	if err := validation.Default().Struct(dst); err != nil {
		var verr *validation.Error
		if stderrors.As(err, &verr) {
			return errors.Wrap(verr, errors.CodeValidation, "validation failed")
		}
		return errors.Wrap(err, errors.CodeInvalidArgument, "validation failed")
	}
	return nil
}
