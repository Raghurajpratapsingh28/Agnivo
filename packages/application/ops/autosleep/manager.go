// Package autosleep implements automatic project sleeping: idle detection,
// plan-specific thresholds, grace periods, sleep scheduling, and wake-on-request.
package autosleep

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpjobs"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"go.uber.org/zap"
)

// IdleThresholds defines how long a project must be inactive before sleeping.
type IdleThresholds struct {
	Free       time.Duration
	Pro        time.Duration
	Team       time.Duration
	Enterprise time.Duration
}

// DefaultThresholds returns sensible defaults.
func DefaultThresholds() IdleThresholds {
	return IdleThresholds{
		Free:       30 * time.Minute,
		Pro:        2 * time.Hour,
		Team:       6 * time.Hour,
		Enterprise: -1, // never auto-sleep
	}
}

// Manager orchestrates auto-sleep and auto-wake operations.
type Manager struct {
	repo       *store.Repository
	queue      *jobs.Queue
	thresholds IdleThresholds
	log        *zap.Logger
}

// NewManager constructs an auto-sleep Manager.
func NewManager(repo *store.Repository, queue *jobs.Queue, thresholds IdleThresholds, log *zap.Logger) *Manager {
	return &Manager{repo: repo, queue: queue, thresholds: thresholds, log: log}
}

// EnqueueSleep enqueues a sleep job for a deployment with the plan-appropriate delay.
func (m *Manager) EnqueueSleep(ctx context.Context, orgID, projectID, deploymentID string, planID model.PlanID, correlationID string) error {
	threshold := m.thresholdFor(planID)
	if threshold < 0 {
		return nil // enterprise: never sleep
	}
	_, err := m.queue.Enqueue(ctx, QueueAutosleep, TypeSleep, model.SleepPayload{
		OrgID:         orgID,
		ProjectID:     projectID,
		DeploymentID:  deploymentID,
		CorrelationID: correlationID,
	}, jobs.EnqueueOptions{
		Delay:          threshold,
		IdempotencyKey: "sleep:" + deploymentID,
		MaxAttempts:    3,
	})
	if err != nil {
		m.log.Warn("autosleep: enqueue failed",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
	}
	return err
}

// ExecuteSleep performs the sleep: records the event, notifies the scheduler.
func (m *Manager) ExecuteSleep(ctx context.Context, p model.SleepPayload) error {
	if err := m.repo.RecordSleepEvent(ctx, model.SleepEvent{
		OrgID:         p.OrgID,
		ProjectID:     p.ProjectID,
		DeploymentID:  p.DeploymentID,
		Status:        model.SleepStatusSleeping,
		Reason:        "idle_threshold_exceeded",
		CorrelationID: p.CorrelationID,
	}); err != nil {
		return err
	}

	// Enqueue the scheduler sleep job via cpjobs.
	_, err := m.queue.Enqueue(ctx, cpjobs.QueueDeployments, cpjobs.TypeSleep, cpjobs.Payload{
		OrgID:         p.OrgID,
		ProjectID:     p.ProjectID,
		DeploymentID:  p.DeploymentID,
		CorrelationID: p.CorrelationID,
	}, jobs.EnqueueOptions{
		IdempotencyKey: "sched-sleep:" + p.DeploymentID,
		MaxAttempts:    5,
	})
	if err != nil {
		m.log.Warn("autosleep: scheduler sleep enqueue failed",
			zap.String("deployment_id", p.DeploymentID),
			zap.Error(err))
	}

	m.log.Info("autosleep: project sleeping",
		zap.String("project_id", p.ProjectID),
		zap.String("deployment_id", p.DeploymentID))
	return nil
}

// ExecuteWake records a wake event and enqueues the scheduler wake job.
func (m *Manager) ExecuteWake(ctx context.Context, orgID, projectID, deploymentID, correlationID string) error {
	if err := m.repo.RecordSleepEvent(ctx, model.SleepEvent{
		OrgID:         orgID,
		ProjectID:     projectID,
		DeploymentID:  deploymentID,
		Status:        model.SleepStatusWaking,
		Reason:        "wake_request",
		CorrelationID: correlationID,
	}); err != nil {
		return err
	}

	_, err := m.queue.Enqueue(ctx, cpjobs.QueueDeployments, cpjobs.TypeWake, cpjobs.Payload{
		OrgID:         orgID,
		ProjectID:     projectID,
		DeploymentID:  deploymentID,
		CorrelationID: correlationID,
	}, jobs.EnqueueOptions{
		IdempotencyKey: "sched-wake:" + deploymentID,
		MaxAttempts:    5,
	})

	m.log.Info("autosleep: wake requested",
		zap.String("project_id", projectID),
		zap.String("deployment_id", deploymentID))
	return err
}

func (m *Manager) thresholdFor(planID model.PlanID) time.Duration {
	switch planID {
	case model.PlanFree:
		return m.thresholds.Free
	case model.PlanPro:
		return m.thresholds.Pro
	case model.PlanTeam:
		return m.thresholds.Team
	case model.PlanEnterprise:
		return m.thresholds.Enterprise
	default:
		return m.thresholds.Free
	}
}

const (
	QueueAutosleep = "autosleep"
	TypeSleep      = "autosleep.sleep"
)
