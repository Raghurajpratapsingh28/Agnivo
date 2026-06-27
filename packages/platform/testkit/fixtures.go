package testkit

import (
	"context"
	"os"
	"testing"
	"time"
)

// Suite holds shared test resources with coordinated setup and teardown.
type Suite struct {
	T       testing.TB
	Context context.Context
	Cancel  context.CancelFunc
}

// NewSuite returns a Suite with a context cancelled when the test finishes.
func NewSuite(t testing.TB, timeout time.Duration) *Suite {
	t.Helper()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return &Suite{T: t, Context: ctx, Cancel: cancel}
}

// SkipUnlessEnv skips the test when the named environment variable is unset.
func SkipUnlessEnv(t testing.TB, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("%s not set", name)
	}
	return v
}
