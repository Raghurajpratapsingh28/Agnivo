package analytics_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/analytics"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewAggregator(t *testing.T) {
	agg := analytics.NewAggregator(nil, nil, zap.NewNop())
	assert.NotNil(t, agg)
}
