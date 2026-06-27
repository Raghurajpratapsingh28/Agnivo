package httpx_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformErrorMapping(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	httpx.Error(rec, req, errors.NotFound("missing"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"not_found"`)
}

func TestNegotiateJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")
	ct, ok := httpx.Negotiate(req, httpx.MediaJSON, httpx.MediaHTML)
	assert.True(t, ok)
	assert.Equal(t, httpx.MediaJSON, ct)
}

func TestParseCursor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?cursor=abc&limit=999", nil)
	p := httpx.ParseCursor(req)
	assert.Equal(t, "abc", p.After)
	assert.Equal(t, httpx.MaxCursorLimit, p.Limit)
}

func TestVersionFromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Version", "v2")
	assert.Equal(t, "2", httpx.VersionFromRequest(req))
}

func TestAttachmentBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	err := httpx.AttachmentBytes(rec, "report.csv", "text/csv", []byte("a,b,c"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "report.csv")
	assert.Equal(t, "a,b,c", rec.Body.String())
}

func TestStreamNDJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	err := httpx.StreamNDJSON(rec, func(yield func(any) error) error {
		if err := yield(map[string]int{"n": 1}); err != nil {
			return err
		}
		return yield(map[string]int{"n": 2})
	})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	assert.Len(t, lines, 2)
}

func TestClientIPForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	assert.Equal(t, "1.2.3.4", httpx.ClientIP(req))
}

func TestCursorPageEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.CursorPage(rec, []int{1, 2}, httpx.NewCursorMeta("next", true))
	assert.Contains(t, rec.Body.String(), `"next_cursor":"next"`)
	assert.Contains(t, rec.Body.String(), `"has_more":true`)
}

func TestReadFormFile(t *testing.T) {
	body := &bytes.Buffer{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", body)
	// without multipart setup, FormFile should error
	_, _, err := httpx.ReadFormFile(req, "file", 1024)
	require.Error(t, err)
	_ = w
}
