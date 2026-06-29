package ecr_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/ecr"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildTags(t *testing.T) {
	tags := ecr.BuildTags("abcdef1234567890", "dep-uuid-1234", "main", true)
	assert.Contains(t, tags, "abcdef123456")
	assert.Contains(t, tags, "dep-dep-uuid")
	assert.Contains(t, tags, "branch-main")
	assert.Contains(t, tags, "latest")
}

func TestRepositoryName(t *testing.T) {
	c := ecr.NewClient(config.ECRConfig{RepositoryPrefix: "agnivo"}, "docker")
	name := c.RepositoryName("org-uuid-12345678", "my-app")
	assert.Equal(t, "agnivo/org-uuid/my-app", name)
}

func TestFullImageRef(t *testing.T) {
	c := ecr.NewClient(config.ECRConfig{
		Enabled: true, Registry: "123.dkr.ecr.us-east-1.amazonaws.com",
	}, "docker")
	ref := c.FullImageRef("agnivo/org/app", "v1")
	assert.Equal(t, "123.dkr.ecr.us-east-1.amazonaws.com/agnivo/org/app:v1", ref)
}

func TestDisabledPush(t *testing.T) {
	c := ecr.NewClient(config.ECRConfig{Enabled: false}, "docker")
	res, err := c.Push(t.Context(), ecr.PushOptions{
		LocalTag: "local:tag", RemoteTags: []string{"v1"}, Repository: "repo",
	})
	assert.NoError(t, err)
	assert.Equal(t, "local", res.Registry)
}
