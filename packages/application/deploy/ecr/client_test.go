package ecr_test

import (
	"testing"

	deployecr "github.com/agnivo/agnivo/packages/application/deploy/ecr"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
)

func TestNewClientDisabled(t *testing.T) {
	c := deployecr.NewClient(config.ECRConfig{Enabled: false}, "docker")
	assert.NoError(t, c.Login(t.Context()))
	exists, err := c.ImageExists(t.Context(), "repo", "latest")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalImageName(t *testing.T) {
	assert.Equal(t, "agnivo-deploy:dep-123", deployecr.LocalImageName("dep-123"))
}

func TestPullEmptyRef(t *testing.T) {
	p := deployecr.NewPuller(config.ECRConfig{}, "docker")
	_, err := p.Pull(t.Context(), deployecr.PullOptions{})
	assert.Error(t, err)
}
