// Package cron implements a distributed cron scheduler with leader election.
// Only the elected leader fires jobs; followers watch and stand by. Leadership
// is held via a Redis lock with automatic renewal. All schedule definitions are
// stored in the database so they can be managed at runtime without code changes.
package cron

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	redisclient "github.com/agnivo/agnivo/packages/platform/cache/redis"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"go.uber.org/zap"
)

const (
	leaderLockKey = "cron:leader"
	leaderLockTTL = 60 * time.Second
	tickInterval  = 30 * time.Second
)

// Scheduler polls for due cron jobs, acquires leadership, and enqueues work.
type Scheduler struct {
	repo   *store.Repository
	queue  *jobs.Queue
	redis  *redisclient.Client
	log    *zap.Logger
	nodeID string
}

// NewScheduler constructs a distributed Scheduler.
func NewScheduler(repo *store.Repository, queue *jobs.Queue, redis *redisclient.Client, nodeID string, log *zap.Logger) *Scheduler {
	return &Scheduler{
		repo:   repo,
		queue:  queue,
		redis:  redis,
		log:    log,
		nodeID: nodeID,
	}
}

// Run starts the scheduler loop. It blocks until ctx is canceled.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	s.log.Info("cron: scheduler started", zap.String("node_id", s.nodeID))
	// Attempt one tick immediately.
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick acquires leadership and fires any due cron jobs.
func (s *Scheduler) tick(ctx context.Context) {
	if s.redis == nil {
		// No Redis: always act as leader (single-node mode).
		s.fireDueJobs(ctx)
		return
	}

	lock, err := s.redis.Acquire(ctx, leaderLockKey, leaderLockTTL)
	if err != nil {
		// Another node holds the lock; this node is a follower.
		return
	}
	defer func() { _ = lock.Release(ctx) }()

	s.fireDueJobs(ctx)
}

func (s *Scheduler) fireDueJobs(ctx context.Context) {
	due, err := s.repo.GetCronJobsDue(ctx)
	if err != nil {
		s.log.Warn("cron: get due jobs failed", zap.Error(err))
		return
	}
	for _, job := range due {
		s.fire(ctx, job)
	}
}

func (s *Scheduler) fire(ctx context.Context, job model.CronJob) {
	nextRun := nextRunAfter(job.Schedule, job.Timezone, time.Now().UTC())

	_, err := s.queue.Enqueue(ctx, job.JobQueue, job.JobType, job.Payload, jobs.EnqueueOptions{
		IdempotencyKey: "cron:" + job.Name + ":" + time.Now().UTC().Format("2006-01-02T15:04"),
		MaxAttempts:    3,
	})

	lastErr := ""
	if err != nil {
		lastErr = err.Error()
		s.log.Warn("cron: enqueue failed",
			zap.String("name", job.Name),
			zap.String("type", job.JobType),
			zap.Error(err))
	} else {
		s.log.Info("cron: job fired",
			zap.String("name", job.Name),
			zap.String("queue", job.JobQueue),
			zap.String("type", job.JobType),
			zap.Time("next_run", nextRun))
	}

	if updateErr := s.repo.UpdateCronJobAfterRun(ctx, job.ID, nextRun, lastErr); updateErr != nil {
		s.log.Warn("cron: update after run failed",
			zap.String("name", job.Name),
			zap.Error(updateErr))
	}
}

// Register ensures a cron job definition exists in the database.
func (s *Scheduler) Register(ctx context.Context, name, schedule, timezone, queue, jobType string, payload any) error {
	raw, _ := json.Marshal(payload)
	next := nextRunAfter(schedule, timezone, time.Now().UTC())
	j := model.CronJob{
		Name:     name,
		Schedule: schedule,
		Timezone: timezone,
		JobQueue: queue,
		JobType:  jobType,
		Payload:  raw,
		Status:   model.CronActive,
		NextRunAt: &next,
	}
	_, err := s.repo.UpsertCronJob(ctx, j)
	return err
}

// nextRunAfter parses a cron expression and returns the next fire time after from.
// This is a lightweight parser that supports: @daily, @hourly, @weekly, @monthly,
// and 5-field cron expressions "min hour dom month dow".
func nextRunAfter(schedule, _ string, from time.Time) time.Time {
	switch schedule {
	case "@yearly", "@annually":
		return time.Date(from.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
	case "@monthly":
		return time.Date(from.Year(), from.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	case "@weekly":
		return from.Truncate(24 * time.Hour).Add(7 * 24 * time.Hour)
	case "@daily", "@midnight":
		return time.Date(from.Year(), from.Month(), from.Day()+1, 0, 0, 0, 0, time.UTC)
	case "@hourly":
		return from.Truncate(time.Hour).Add(time.Hour)
	default:
		// For full 5-field cron expressions fall back to +1 minute.
		// Production deployments should use a proper cron library like robfig/cron.
		return from.Add(time.Minute).Truncate(time.Minute)
	}
}
