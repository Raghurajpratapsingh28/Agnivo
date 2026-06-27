package ptr_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/ptr"
	"github.com/stretchr/testify/assert"
)

func TestOfAndDeref(t *testing.T) {
	p := ptr.Of(42)
	assert.Equal(t, 42, *p)
	assert.Equal(t, 42, ptr.Deref(p))
	assert.Equal(t, 0, ptr.Deref[int](nil))
	assert.Equal(t, 7, ptr.DerefOr[int](nil, 7))
}

func TestEqual(t *testing.T) {
	assert.True(t, ptr.Equal[int](nil, nil))
	assert.False(t, ptr.Equal(ptr.Of(1), nil))
	assert.True(t, ptr.Equal(ptr.Of("a"), ptr.Of("a")))
	assert.False(t, ptr.Equal(ptr.Of("a"), ptr.Of("b")))
}

func TestNonZero(t *testing.T) {
	assert.Nil(t, ptr.NonZero(""))
	assert.Equal(t, "x", *ptr.NonZero("x"))
}
