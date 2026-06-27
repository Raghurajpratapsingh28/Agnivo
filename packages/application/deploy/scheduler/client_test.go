package scheduler_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/scheduler"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalReserve(t *testing.T) {
	client := scheduler.NewClient(config.Deployer{})
	placement, err := client.Reserve(t.Context(), "o1", "p1", "d1", 30000, 40000)
	require.NoError(t, err)
	assert.True(t, placement.Reserved)
	assert.Equal(t, "127.0.0.1", placement.Host)
	assert.Equal(t, 30000, placement.Port)
}

func TestLocalRelease(t *testing.T) {
	client := scheduler.NewClient(config.Deployer{})
	assert.NoError(t, client.Release(t.Context(), "o1", "p1", "d1"))
}

func TestFormatHostPort(t *testing.T) {
	assert.Equal(t, "127.0.0.1:8080", scheduler.FormatHostPort(scheduler.Placement{Host: "127.0.0.1", Port: 8080}))
}
