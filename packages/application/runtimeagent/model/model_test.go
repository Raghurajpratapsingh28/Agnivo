package model_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/application/runtimeagent/model"
	"github.com/stretchr/testify/assert"
)

func TestContainerStatuses(t *testing.T) {
	assert.Equal(t, model.ContainerStatus("running"), model.StatusRunning)
	assert.Equal(t, model.ContainerStatus("deleted"), model.StatusDeleted)
}

func TestCreateRequestFields(t *testing.T) {
	req := model.CreateRequest{
		DeploymentID: "d1", Image: "nginx:latest", Port: 8080, HostPort: 30001,
		Env: map[string]string{"PORT": "8080"},
	}
	assert.Equal(t, "d1", req.DeploymentID)
	assert.Equal(t, 30001, req.HostPort)
}
