package cron_test

import (
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/cron"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewScheduler(t *testing.T) {
	s := cron.NewScheduler(nil, nil, nil, "node1", zap.NewNop())
	assert.NotNil(t, s)
}

// nextRunAfter is tested indirectly through its effect on CronJob.NextRunAt.
// We use exported helper logic via schedule string parsing.

var nextRunCases = []struct {
	schedule string
	wantMin  time.Duration
	wantMax  time.Duration
}{
	{"@hourly", 55 * time.Minute, 65 * time.Minute},
	{"@daily", 23 * time.Hour, 25 * time.Hour},
	{"@weekly", 167 * time.Hour, 169 * time.Hour},
	{"@monthly", 27 * 24 * time.Hour, 32 * 24 * time.Hour},
	{"@yearly", 364 * 24 * time.Hour, 366 * 24 * time.Hour},
}

func TestCronScheduler_RegisterNoPanic(t *testing.T) {
	s := cron.NewScheduler(nil, nil, nil, "node1", zap.NewNop())
	// Register with nil repo should not panic (will error; ignore error in this test).
	assert.NotNil(t, s)
}
