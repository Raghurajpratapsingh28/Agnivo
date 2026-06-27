package strx_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/platform/strx"
	"github.com/stretchr/testify/assert"
)

func TestSlugify(t *testing.T) {
	assert.Equal(t, "hello-world", strx.Slugify("Hello, World!"))
	assert.Equal(t, "a-b-c", strx.Slugify("  a   b   c  "))
	assert.Equal(t, "my-app-v2", strx.Slugify("My App (v2)"))
	assert.Equal(t, "", strx.Slugify("!!!"))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", strx.Truncate("hello", 10))
	assert.Equal(t, "hel\u2026", strx.Truncate("hello world", 4))
	assert.Equal(t, "", strx.Truncate("hello", 0))
}

func TestBlankAndCoalesce(t *testing.T) {
	assert.True(t, strx.IsBlank("   "))
	assert.False(t, strx.IsBlank(" x "))
	assert.Equal(t, "b", strx.Coalesce("", "  ", "b", "c"))
	assert.Equal(t, "", strx.Coalesce("", " "))
}

func TestToSnake(t *testing.T) {
	assert.Equal(t, "user_id", strx.ToSnake("UserID"))
	assert.Equal(t, "http_server", strx.ToSnake("httpServer"))
}

func TestRedact(t *testing.T) {
	assert.Equal(t, "*******789a", strx.Redact("0123456789a", 4))
	assert.Equal(t, "****", strx.Redact("abcd", 4))
	assert.Equal(t, "*234", strx.Redact("1234", 3))
}
