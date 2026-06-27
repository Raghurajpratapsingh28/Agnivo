package testkit

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// Logger returns a logger that writes to the test log.
func Logger(t testing.TB) *zap.Logger { return zaptest.NewLogger(t) }

// NopLogger returns a no-op logger.
func NopLogger() *zap.Logger { return logger.NewNop() }

// Context returns a context that is cancelled when the test finishes or after
// the given timeout, whichever comes first.
func Context(t testing.TB, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// FreePort returns an available TCP port for tests that bind servers.
func FreePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testkit: free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}
