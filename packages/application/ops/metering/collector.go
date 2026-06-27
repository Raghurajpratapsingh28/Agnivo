// Package metering tracks resource consumption and generates usage rollups.
package metering

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// Collector records usage measurements and drives rollup aggregation.
type Collector struct {
	repo *store.Repository
	log  *zap.Logger
}

// NewCollector creates a usage Collector.
func NewCollector(repo *store.Repository, log *zap.Logger) *Collector {
	return &Collector{repo: repo, log: log}
}

// Record persists a single usage event. quantity must be > 0.
func (c *Collector) Record(ctx context.Context, orgID, projectID, deploymentID string, dimension model.UsageDimension, quantity float64, correlationID string) error {
	if quantity <= 0 {
		return errors.New(errors.CodeInvalidArgument, "metering: quantity must be positive")
	}
	period := today()
	unit := unitFor(dimension)
	return c.repo.RecordUsage(ctx, model.UsageRecord{
		OrgID:         orgID,
		ProjectID:     projectID,
		DeploymentID:  deploymentID,
		Dimension:     dimension,
		Quantity:      quantity,
		Unit:          unit,
		Period:        period,
		CorrelationID: correlationID,
	})
}

// RecordBatch records multiple usage events in one call.
func (c *Collector) RecordBatch(ctx context.Context, records []model.UsageRecord) error {
	for _, r := range records {
		if r.Period == "" {
			r.Period = today()
		}
		if r.Unit == "" {
			r.Unit = unitFor(r.Dimension)
		}
		if err := c.repo.RecordUsage(ctx, r); err != nil {
			c.log.Warn("metering: batch record failed",
				zap.String("org_id", r.OrgID),
				zap.String("dimension", string(r.Dimension)),
				zap.Error(err))
		}
	}
	return nil
}

// Rollup aggregates raw usage records into daily rollups for a given period.
func (c *Collector) Rollup(ctx context.Context, period string) error {
	if period == "" {
		period = today()
	}
	if err := c.repo.RollupUsage(ctx, period); err != nil {
		return err
	}
	c.log.Info("metering: rollup complete", zap.String("period", period))
	return nil
}

// CurrentUsage returns the current period's total for an org/dimension.
func (c *Collector) CurrentUsage(ctx context.Context, orgID string, dimension model.UsageDimension) (float64, error) {
	return c.repo.GetCurrentUsage(ctx, orgID, dimension, today())
}

// RollupYesterday runs the rollup for the previous day — useful for nightly cron.
func (c *Collector) RollupYesterday(ctx context.Context) error {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	return c.Rollup(ctx, yesterday)
}

// unitFor returns the canonical unit string for a dimension.
func unitFor(d model.UsageDimension) string {
	switch d {
	case model.DimBandwidthGB, model.DimStorageGB, model.DimLogGB, model.DimMemoryGBHours:
		return "GB"
	case model.DimBuildMinutes, model.DimContainerHours, model.DimCPUCoreHours:
		return "hours"
	case model.DimRequests, model.DimAPIRequests, model.DimDeployments,
		model.DimCustomDomains, model.DimSSLCerts, model.DimProjects,
		model.DimWSConnections, model.DimStreamingSessions:
		return "count"
	default:
		return "unit"
	}
}

func today() string {
	return time.Now().UTC().Format("2006-01-02")
}
