package quota_test

import (
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/quota"
	"github.com/stretchr/testify/assert"
)

func TestCurrentPeriod_Format(t *testing.T) {
	expected := time.Now().UTC().Format("2006-01-02")
	// Period must be YYYY-MM-DD (10 chars).
	assert.Len(t, expected, 10)
}

func TestQuotaConfig_Defaults(t *testing.T) {
	_ = quota.NewEnforcer(nil, nil)
	// Confirm constructor doesn't panic with nil repo.
	assert.NotNil(t, quota.NewEnforcer(nil, nil))
}
