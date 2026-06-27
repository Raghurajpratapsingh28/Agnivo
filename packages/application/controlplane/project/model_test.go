package project_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/project"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeSlug(t *testing.T) {
	assert.Equal(t, "my-app", project.NormalizeSlug("My App"))
	assert.Equal(t, "hello-world", project.NormalizeSlug("Hello World!"))
}

func TestProjectIsLive(t *testing.T) {
	p := project.Project{Status: project.StatusActive}
	assert.True(t, p.IsLive())
	p.Status = project.StatusArchived
	assert.False(t, p.IsLive())
}

func TestStatusConstants(t *testing.T) {
	assert.Equal(t, project.Status("active"), project.StatusActive)
	assert.Equal(t, project.Visibility("private"), project.VisibilityPrivate)
}
