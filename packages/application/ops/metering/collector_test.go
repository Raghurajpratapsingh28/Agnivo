package metering_test

import (
	"context"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/metering"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRecord_NegativeQuantity(t *testing.T) {
	c := metering.NewCollector(nil, zap.NewNop())
	ctx := context.Background()
	err := c.Record(ctx, "org1", "proj1", "dep1", model.DimDeployments, -1, "corr1")
	require.Error(t, err)
}

func TestRecord_ZeroQuantity(t *testing.T) {
	c := metering.NewCollector(nil, zap.NewNop())
	ctx := context.Background()
	err := c.Record(ctx, "org1", "proj1", "dep1", model.DimBandwidthGB, 0, "corr1")
	require.Error(t, err)
}

func TestCurrentPeriodFormat(t *testing.T) {
	period := time.Now().UTC().Format("2006-01-02")
	assert.Len(t, period, 10, "period should be YYYY-MM-DD")
}

func TestRecordBatch_EmptyDoesNotError(t *testing.T) {
	c := metering.NewCollector(nil, zap.NewNop())
	ctx := context.Background()
	require.NoError(t, c.RecordBatch(ctx, nil))
}

func TestDimensions(t *testing.T) {
	dims := []model.UsageDimension{
		model.DimDeployments, model.DimBuildMinutes, model.DimContainerHours,
		model.DimBandwidthGB, model.DimStorageGB, model.DimLogGB,
		model.DimCPUCoreHours, model.DimMemoryGBHours, model.DimRequests,
		model.DimCustomDomains, model.DimSSLCerts, model.DimProjects,
		model.DimAPIRequests, model.DimWSConnections, model.DimStreamingSessions,
	}
	for _, d := range dims {
		assert.NotEmpty(t, string(d))
	}
}
