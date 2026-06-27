package metrics_test

import (
	"testing"

	opsmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_Collectors(t *testing.T) {
	m := opsmetrics.New("test-worker")
	collectors := m.Collectors()
	assert.NotEmpty(t, collectors)

	reg := prometheus.NewRegistry()
	for _, c := range collectors {
		require.NoError(t, reg.Register(c))
	}
}
