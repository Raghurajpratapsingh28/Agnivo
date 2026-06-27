package audit_test

import (
	"context"
	"testing"

	"github.com/agnivo/agnivo/packages/application/ops/audit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	l := audit.NewLogger(nil, "worker", zap.NewNop())
	assert.NotNil(t, l)
}

func TestLogger_RecordDoesNotPanic_NilRepo(t *testing.T) {
	l := audit.NewLogger(nil, "worker", zap.NewNop())
	// Record should not panic even with nil repo (it logs the error internally).
	assert.NotPanics(t, func() {
		l.Record(context.Background(), audit.RecordInput{
			OrgID:        "org1",
			Action:       "deployment.created",
			ResourceType: "deployment",
			ResourceID:   "dep1",
		})
	})
}

func TestActorTypes(t *testing.T) {
	assert.Equal(t, "user", audit.ActorUser)
	assert.Equal(t, "system", audit.ActorSystem)
	assert.Equal(t, "api_key", audit.ActorAPIKey)
}
