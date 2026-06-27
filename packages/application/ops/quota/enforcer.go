// Package quota enforces resource limits per plan.
// Every check returns either nil (allowed), a soft violation (warn), or a
// hard violation (block) — callers decide how to surface each.
package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// Enforcer evaluates quota limits for an org's active plan.
type Enforcer struct {
	repo *store.Repository
	log  *zap.Logger
}

// NewEnforcer constructs a quota Enforcer.
func NewEnforcer(repo *store.Repository, log *zap.Logger) *Enforcer {
	return &Enforcer{repo: repo, log: log}
}

// CheckResult describes the outcome of a quota check.
type CheckResult struct {
	Allowed    bool
	Violations []model.QuotaViolation
}

// Check evaluates all quota dimensions for an org and returns any violations.
func (e *Enforcer) Check(ctx context.Context, orgID string, dimension model.UsageDimension, delta float64) (CheckResult, error) {
	sub, err := e.repo.GetSubscription(ctx, orgID)
	if err != nil {
		// Unknown org → default to free limits.
		sub.PlanID = model.PlanFree
	}

	cfg, err := e.repo.GetQuotaConfig(ctx, sub.PlanID)
	if err != nil {
		return CheckResult{Allowed: true}, nil // Fail open; log below.
	}

	limit := limitFor(cfg, dimension)
	if limit < 0 {
		// -1 means unlimited (enterprise).
		return CheckResult{Allowed: true}, nil
	}
	if limit == 0 {
		return CheckResult{
			Allowed: false,
			Violations: []model.QuotaViolation{{
				OrgID: orgID, Dimension: dimension,
				Current: delta, Limit: 0, Pct: 100, IsHard: true,
			}},
		}, nil
	}

	current, err := e.repo.GetCurrentUsage(ctx, orgID, dimension, currentPeriod())
	if err != nil {
		return CheckResult{Allowed: true}, nil
	}

	projected := current + delta
	pct := (projected / float64(limit)) * 100

	v := model.QuotaViolation{
		OrgID:     orgID,
		Dimension: dimension,
		Current:   projected,
		Limit:     float64(limit),
		Pct:       pct,
	}

	if pct > 100 {
		v.IsHard = true
		e.log.Warn("quota: hard limit exceeded",
			zap.String("org_id", orgID),
			zap.String("dimension", string(dimension)),
			zap.Float64("pct", pct))
		return CheckResult{Allowed: false, Violations: []model.QuotaViolation{v}}, nil
	}

	warnPct := cfg.WarnThresholdPct
	if warnPct <= 0 {
		warnPct = 80
	}
	if pct >= warnPct {
		e.log.Info("quota: warning threshold reached",
			zap.String("org_id", orgID),
			zap.String("dimension", string(dimension)),
			zap.Float64("pct", pct))
		return CheckResult{Allowed: true, Violations: []model.QuotaViolation{v}}, nil
	}

	return CheckResult{Allowed: true}, nil
}

// Enforce is like Check but returns an error if the hard limit is exceeded.
func (e *Enforcer) Enforce(ctx context.Context, orgID string, dimension model.UsageDimension, delta float64) error {
	result, err := e.Check(ctx, orgID, dimension, delta)
	if err != nil {
		return err
	}
	if !result.Allowed {
		for _, v := range result.Violations {
			if v.IsHard {
				return errors.New(errors.CodeExhausted,
					fmt.Sprintf("quota exceeded: %s at %.1f%% of plan limit", dimension, v.Pct))
			}
		}
	}
	return nil
}

// SummaryForOrg returns current usage vs limits for all key dimensions.
func (e *Enforcer) SummaryForOrg(ctx context.Context, orgID string) (map[string]model.QuotaViolation, error) {
	sub, err := e.repo.GetSubscription(ctx, orgID)
	if err != nil {
		sub.PlanID = model.PlanFree
	}
	cfg, err := e.repo.GetQuotaConfig(ctx, sub.PlanID)
	if err != nil {
		return nil, err
	}

	dims := []model.UsageDimension{
		model.DimDeployments, model.DimBuildMinutes, model.DimContainerHours,
		model.DimBandwidthGB, model.DimStorageGB, model.DimCustomDomains,
	}

	summary := make(map[string]model.QuotaViolation, len(dims))
	period := currentPeriod()
	for _, d := range dims {
		current, _ := e.repo.GetCurrentUsage(ctx, orgID, d, period)
		limit := limitFor(cfg, d)
		pct := 0.0
		if limit > 0 {
			pct = (current / float64(limit)) * 100
		}
		summary[string(d)] = model.QuotaViolation{
			OrgID:     orgID,
			Dimension: d,
			Current:   current,
			Limit:     float64(limit),
			Pct:       pct,
			IsHard:    pct > 100,
		}
	}
	return summary, nil
}

// ─────────────────────────────── Helpers ─────────────────────────────────────

func limitFor(cfg model.QuotaConfig, d model.UsageDimension) float64 {
	switch d {
	case model.DimProjects:
		return float64(cfg.MaxProjects)
	case model.DimDeployments:
		return float64(cfg.MaxDeployments)
	case model.DimBuildMinutes:
		return cfg.MaxBuildMinutesMonth
	case model.DimContainerHours:
		return cfg.MaxContainerHoursMonth
	case model.DimBandwidthGB:
		return cfg.MaxBandwidthGBMonth
	case model.DimStorageGB:
		return cfg.MaxStorageGB
	case model.DimCustomDomains:
		return float64(cfg.MaxCustomDomains)
	case model.DimAPIRequests:
		return float64(cfg.MaxAPIRequestsDay)
	default:
		return -1 // unlimited for unknown dimensions
	}
}

func currentPeriod() string {
	return time.Now().UTC().Format("2006-01-02")
}
