package deployment_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	"github.com/stretchr/testify/assert"
)

func TestDeploymentTerminalStates(t *testing.T) {
	assert.True(t, deployment.Deployment{Status: deployment.StatusLive}.IsTerminal())
	assert.True(t, deployment.Deployment{Status: deployment.StatusFailed}.IsTerminal())
	assert.False(t, deployment.Deployment{Status: deployment.StatusBuilding}.IsTerminal())
	assert.True(t, deployment.Deployment{Status: deployment.StatusBuilding}.IsActive())
	assert.False(t, deployment.Deployment{Status: deployment.StatusPending}.IsActive())
}
