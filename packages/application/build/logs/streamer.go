package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildstore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
)

// Level is a log severity.
type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelDebug Level = "debug"
)

// Streamer persists and streams build logs.
type Streamer struct {
	repo  *buildstore.LogRepository
	redis *redis.Client
}

// NewStreamer constructs a log streamer.
func NewStreamer(repo *buildstore.LogRepository, redis *redis.Client) *Streamer {
	return &Streamer{repo: repo, redis: redis}
}

// LogChannel returns the Redis pub/sub channel for a deployment's build logs.
func LogChannel(deploymentID string) string {
	return fmt.Sprintf("build:logs:%s", deploymentID)
}

// Entry is a structured log line emitted during builds.
type Entry struct {
	BuildID      string
	DeploymentID string
	Stage        string
	Level        Level
	Message      string
	Metadata     map[string]any
	Timestamp    time.Time
}

// Write persists and publishes a log entry. Credentials are never logged.
func (s *Streamer) Write(ctx context.Context, e Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	meta, _ := json.Marshal(e.Metadata)
	entry := model.LogEntry{
		BuildID: e.BuildID, DeploymentID: e.DeploymentID, Stage: e.Stage,
		Level: string(e.Level), Message: sanitize(e.Message), Metadata: meta,
	}
	if err := s.repo.Append(ctx, entry); err != nil {
		return err
	}
	if s.redis == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"build_id": e.BuildID, "deployment_id": e.DeploymentID, "stage": e.Stage,
		"level": e.Level, "message": sanitize(e.Message), "metadata": e.Metadata,
		"timestamp": e.Timestamp,
	})
	_, _ = s.redis.Publish(ctx, LogChannel(e.DeploymentID), payload)
	return nil
}

// Info logs an info message.
func (s *Streamer) Info(ctx context.Context, buildID, deploymentID, stage, msg string) error {
	return s.Write(ctx, Entry{BuildID: buildID, DeploymentID: deploymentID, Stage: stage, Level: LevelInfo, Message: msg})
}

// Warn logs a warning.
func (s *Streamer) Warn(ctx context.Context, buildID, deploymentID, stage, msg string) error {
	return s.Write(ctx, Entry{BuildID: buildID, DeploymentID: deploymentID, Stage: stage, Level: LevelWarn, Message: msg})
}

// Error logs an error.
func (s *Streamer) Error(ctx context.Context, buildID, deploymentID, stage, msg string) error {
	return s.Write(ctx, Entry{BuildID: buildID, DeploymentID: deploymentID, Stage: stage, Level: LevelError, Message: msg})
}

// sanitize redacts common credential patterns from log output.
func sanitize(msg string) string {
	patterns := []string{"ghp_", "gho_", "glpat-", "xoxb-", "Bearer ", "password=", "token="}
	out := msg
	for _, p := range patterns {
		if idx := indexOf(out, p); idx >= 0 {
			out = out[:idx] + p + "[REDACTED]"
		}
	}
	return out
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
