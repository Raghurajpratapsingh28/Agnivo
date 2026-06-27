package dto_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOKEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	dto.OK(w, map[string]string{"hello": "world"}, dto.WithRequestID("req-1"))

	assert.Equal(t, http.StatusOK, w.Code)
	var resp dto.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.JSONEq(t, `{"hello":"world"}`, string(resp.Data))
	require.NotNil(t, resp.Meta)
	assert.Equal(t, "req-1", resp.Meta.RequestID)
}

func TestPageEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	dto.Page(w, []int{1, 2, 3}, dto.NewPageMeta(1, 3, 9))

	var resp dto.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Meta.Page)
	assert.Equal(t, int64(9), resp.Meta.Page.TotalItems)
	assert.Equal(t, int64(3), resp.Meta.Page.TotalPages)
}

func TestCursorEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	dto.Cursor(w, []int{1}, dto.CursorMeta{NextCursor: "abc", HasMore: true})
	var resp dto.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Meta.Cursor)
	assert.Equal(t, "abc", resp.Meta.Cursor.NextCursor)
	assert.True(t, resp.Meta.Cursor.HasMore)
}

func TestErrorMapping(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	dto.Error(w, r, errors.NotFound("widget not found"))

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp dto.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, "not_found", resp.Error.Code)
	assert.Equal(t, "widget not found", resp.Error.Message)
}

func TestDecodeValidateSurfacesFieldDetails(t *testing.T) {
	type req struct {
		Name string `json:"name" validate:"required"`
		Slug string `json:"slug" validate:"required,slug"`
	}
	body := `{"name":"","slug":"Bad Slug"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()

	var dst req
	err := dto.DecodeValidate(w, r, &dst)
	require.Error(t, err)
	assert.Equal(t, errors.CodeValidation, errors.CodeOf(err))

	// Rendered error carries field details.
	ew := httptest.NewRecorder()
	dto.Error(ew, r, err)
	assert.Equal(t, http.StatusUnprocessableEntity, ew.Code)
	var resp dto.Response
	require.NoError(t, json.Unmarshal(ew.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.NotNil(t, resp.Error.Details)
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","extra":1}`))
	w := httptest.NewRecorder()
	var dst req
	err := dto.Decode(w, r, &dst)
	require.Error(t, err)
	assert.Equal(t, errors.CodeInvalidArgument, errors.CodeOf(err))
}
