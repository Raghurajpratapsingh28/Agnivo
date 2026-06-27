package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
	"github.com/stretchr/testify/require"
)

func TestOKEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.OK(rec, map[string]string{"hello": "world"})

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":{"hello":"world"}}`, rec.Body.String())
}

func TestErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	httpx.Error(rec, req, httpx.ErrNotFound("project not found"))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":{"code":"not_found","message":"project not found"}}`, rec.Body.String())
}

type createReq struct {
	Name string `json:"name" validate:"required,min=3"`
}

func TestDecodeAndValidate(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok"}`))

	var body createReq
	err := httpx.DecodeAndValidate(rec, req, &body)
	require.Error(t, err) // "ok" is shorter than min=3

	var apiErr *httpx.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnprocessableEntity, apiErr.Status)
}

func TestParsePageClamping(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=0&page_size=9999", nil)
	p := httpx.ParsePage(req)
	require.Equal(t, httpx.DefaultPage, p.Page)
	require.Equal(t, httpx.MaxPageSize, p.PageSize)
	require.Equal(t, 0, p.Offset())
}
