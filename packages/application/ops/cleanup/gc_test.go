package cleanup_test

import (
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/cleanup"
	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := cleanup.DefaultConfig()
	assert.Equal(t, 7*24*time.Hour, cfg.SessionMaxAge)
	assert.Equal(t, 24*time.Hour, cfg.TokenMaxAge)
	assert.Equal(t, 30*24*time.Hour, cfg.LogMaxAge)
	assert.Equal(t, 7*24*time.Hour, cfg.JobDeadLetterAge)
	assert.Equal(t, 90*24*time.Hour, cfg.NotificationMaxAge)
	assert.Equal(t, 365*24*time.Hour, cfg.AuditRetention)
	assert.Equal(t, 90*24*time.Hour, cfg.UsageRawRetention)
}

func TestGC_NilDB_Construction(t *testing.T) {
	gc := cleanup.NewGC(nil, cleanup.DefaultConfig(), nil)
	assert.NotNil(t, gc)
}

func TestTargetConstants(t *testing.T) {
	targets := []cleanup.Target{
		cleanup.TargetExpiredSessions,
		cleanup.TargetExpiredTokens,
		cleanup.TargetOldLogs,
		cleanup.TargetExpiredJobs,
		cleanup.TargetOldNotifications,
		cleanup.TargetExpiredBackups,
		cleanup.TargetStaleUsage,
		cleanup.TargetOldAuditEvents,
	}
	for _, t2 := range targets {
		assert.NotEmpty(t, string(t2))
	}
}
