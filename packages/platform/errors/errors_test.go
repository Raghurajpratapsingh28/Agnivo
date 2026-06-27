package errors_test

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAndCode(t *testing.T) {
	err := errors.New(errors.CodeNotFound, "user not found")
	assert.Equal(t, errors.CodeNotFound, err.Code())
	assert.Equal(t, "user not found", err.Message())
	assert.Equal(t, http.StatusNotFound, err.HTTPStatus())
}

func TestWrapPreservesCause(t *testing.T) {
	sentinel := stderrors.New("db down")
	err := errors.Wrap(sentinel, errors.CodeUnavailable, "query failed")
	require.NotNil(t, err)
	assert.True(t, stderrors.Is(err, sentinel))
	assert.Equal(t, sentinel, stderrors.Unwrap(err))
}

func TestWrapNilReturnsNil(t *testing.T) {
	assert.Nil(t, errors.Wrap(nil, errors.CodeInternal, "x"))
	assert.Nil(t, errors.Wrapf(nil, errors.CodeInternal, "x %d", 1))
	assert.Nil(t, errors.From(nil))
}

func TestIsMatchesByCode(t *testing.T) {
	err := errors.NotFound("missing").WithField("id", "42")
	assert.True(t, errors.Is(err, errors.NotFound("")))
	assert.False(t, errors.Is(err, errors.Conflict("")))
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
}

func TestFromPlainError(t *testing.T) {
	e := errors.From(stderrors.New("boom"))
	require.NotNil(t, e)
	assert.Equal(t, errors.CodeInternal, e.Code())
	assert.Equal(t, http.StatusInternalServerError, e.HTTPStatus())
}

func TestRetryableDefaultsAndOverride(t *testing.T) {
	assert.True(t, errors.RateLimited("slow down").Retryable())
	assert.False(t, errors.NotFound("x").Retryable())
	assert.True(t, errors.NotFound("x").WithRetryable(true).Retryable())
	assert.True(t, errors.IsRetryable(errors.Unavailable("x")))
}

func TestFatal(t *testing.T) {
	err := errors.Internal("corrupt").AsFatal()
	assert.True(t, err.Fatal())
	assert.True(t, errors.IsFatal(err))
	assert.False(t, errors.IsFatal(errors.NotFound("x")))
}

func TestWithFieldsImmutable(t *testing.T) {
	base := errors.NotFound("x")
	enriched := base.WithField("k", "v")
	assert.Empty(t, base.Fields()["k"])
	assert.Equal(t, "v", enriched.Fields()["k"])
	assert.Equal(t, string(errors.CodeNotFound), enriched.Fields()["error_code"])
}

func TestWithOpFormatsError(t *testing.T) {
	err := errors.Wrap(stderrors.New("cause"), errors.CodeConflict, "dup").WithOp("repo.User.Insert")
	assert.Contains(t, err.Error(), "repo.User.Insert")
	assert.Contains(t, err.Error(), "conflict")
	assert.Contains(t, err.Error(), "cause")
}

func TestZapFields(t *testing.T) {
	fields := errors.ZapFields(errors.RateLimited("slow").WithField("ip", "1.2.3.4"))
	keys := map[string]bool{}
	for _, f := range fields {
		keys[f.Key] = true
	}
	assert.True(t, keys["error"])
	assert.True(t, keys["error_code"])
	assert.True(t, keys["retryable"])
	assert.True(t, keys["ip"])
}

func TestErrorsAsTraversesChain(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", errors.NotFound("inner"))
	var e *errors.Error
	require.True(t, errors.As(wrapped, &e))
	assert.Equal(t, errors.CodeNotFound, e.Code())
}
