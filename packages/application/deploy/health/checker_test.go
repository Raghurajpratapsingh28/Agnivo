package health_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/deploy/health"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitUntilHealthySuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	host, _, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port

	checker := health.NewChecker(config.HealthConfig{
		StartupTimeout:   5 * time.Second,
		SuccessThreshold: 1,
		FailureThreshold: 3,
		TCPPort:          port,
	})

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	results, err := checker.WaitUntilHealthy(context.Background(), host, port)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.True(t, results[len(results)-1].Success)
}

func TestLiveness(t *testing.T) {
	checker := health.NewChecker(config.HealthConfig{})
	res := checker.Liveness(context.Background(), "127.0.0.1", 1)
	assert.False(t, res.Success)
}
