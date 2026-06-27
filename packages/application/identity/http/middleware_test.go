package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	idhttp "github.com/agnivo/agnivo/packages/application/identity/http"
	"github.com/agnivo/agnivo/packages/application/identity/jwt"
	"github.com/agnivo/agnivo/packages/application/identity/session"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireAuthMiddleware(t *testing.T) {
	mw := idhttp.NewMiddleware(idhttp.MiddlewareDeps{})
	called := false
	handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Unauthenticated request.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, called)

	// Authenticated request.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(tenant.WithUser(context.Background(), "user-1"))
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestAuthenticateJWTMiddleware(t *testing.T) {
	priv, pub, err := jwt.GenerateKeyPair()
	require.NoError(t, err)
	mgr := jwt.NewManager(jwt.Config{
		PrivateKey: priv, PublicKey: pub,
		Issuer: "test", Audience: "api",
		AccessTTL: time.Minute,
	})
	token, _, err := mgr.IssueAccessToken("user-1", "sess-1", "")
	require.NoError(t, err)

	mw := idhttp.NewMiddleware(idhttp.MiddlewareDeps{
		JWT:        mgr,
		Revocation: session.NewRevocationStore(nil),
	})

	var gotUser string
	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = tenant.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-1", gotUser)
}
