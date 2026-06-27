package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DoJSON executes the handler against a request and decodes the JSON response
// body into out, returning the recorded response for status assertions.
func DoJSON(t testing.TB, h http.Handler, req *http.Request, out any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out), "decode response body")
	}
	return rec
}

// RequireStatus asserts the recorded response has the expected status code.
func RequireStatus(t testing.TB, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	require.Equal(t, want, rec.Code, "unexpected status; body=%s", rec.Body.String())
}

// RequireJSONEq asserts two JSON strings are equal after normalization.
func RequireJSONEq(t testing.TB, expected, actual string) {
	t.Helper()
	require.JSONEq(t, expected, actual)
}

// AssertErrorCode asserts err carries the given platform error code string.
func AssertErrorCode(t testing.TB, err error, code string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), code)
}

// DecodeJSON unmarshals data into dst, failing the test on error.
func DecodeJSON(t testing.TB, data []byte, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(data, dst))
}

// RequireNoError is a thin alias for clarity in test setup blocks.
func RequireNoError(t testing.TB, err error) {
	t.Helper()
	require.NoError(t, err)
}
