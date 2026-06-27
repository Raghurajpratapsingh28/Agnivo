package validation_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandaloneValidators(t *testing.T) {
	assert.True(t, validation.IsUUID("550e8400-e29b-41d4-a716-446655440000"))
	assert.False(t, validation.IsUUID("nope"))

	assert.True(t, validation.IsSlug("my-app-2"))
	assert.False(t, validation.IsSlug("My App"))
	assert.False(t, validation.IsSlug("-bad-"))

	assert.True(t, validation.IsDomain("api.agnivo.io"))
	assert.False(t, validation.IsDomain("not a domain"))

	assert.True(t, validation.IsURL("https://agnivo.io/x"))
	assert.False(t, validation.IsURL("ftp://x"))
	assert.False(t, validation.IsURL("/relative"))

	assert.True(t, validation.IsDockerImage("ghcr.io/agnivo/api:1.2.3"))
	assert.True(t, validation.IsDockerImage("nginx"))
	assert.True(t, validation.IsDockerImage("repo@sha256:"+repeat64()))
	assert.False(t, validation.IsDockerImage("Bad Image"))

	assert.True(t, validation.IsGitRepo("https://github.com/Raghurajpratapsingh28/Agnivo.git"))
	assert.True(t, validation.IsGitRepo("git@github.com:agnivo/agnivo.git"))
	assert.False(t, validation.IsGitRepo("just-text"))

	assert.True(t, validation.IsEnvVarName("DATABASE_URL"))
	assert.False(t, validation.IsEnvVarName("lowercase"))
	assert.False(t, validation.IsEnvVarName("1BAD"))

	assert.True(t, validation.IsStrongSecret("Abcd1234!StrongKey"))
	assert.False(t, validation.IsStrongSecret("short"))
	assert.False(t, validation.IsStrongSecret("alllowercaseletters"))
}

func TestStructTagsRegistered(t *testing.T) {
	type req struct {
		Slug  string `json:"slug" validate:"required,slug"`
		Repo  string `json:"repo" validate:"required,git_repo"`
		Image string `json:"image" validate:"required,docker_image"`
	}
	v := validation.New()

	require.NoError(t, v.Struct(req{Slug: "ok-slug", Repo: "https://github.com/a/b.git", Image: "nginx:1.25"}))

	err := v.Struct(req{Slug: "Bad Slug", Repo: "x", Image: "Bad Image"})
	require.Error(t, err)
	var verr *validation.Error
	require.ErrorAs(t, err, &verr)
	assert.Len(t, verr.Fields, 3)
}

func repeat64() string {
	s := ""
	for i := 0; i < 64; i++ {
		s += "a"
	}
	return s
}
