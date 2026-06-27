package cancel_test

import (
	"context"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/deploy/cancel"
	"github.com/stretchr/testify/assert"
)

func TestCancelRegistry(t *testing.T) {
	r := cancel.NewRegistry()
	ctx, cancelFn := context.WithCancel(context.Background())
	r.Register("dep-1", cancelFn)
	assert.Equal(t, 1, r.ActiveCount())
	assert.True(t, r.Cancel("dep-1"))

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context not cancelled")
	}

	r.Unregister("dep-1")
	assert.Equal(t, 0, r.ActiveCount())
}

func TestCancelUnknown(t *testing.T) {
	r := cancel.NewRegistry()
	assert.False(t, r.Cancel("missing"))
}
