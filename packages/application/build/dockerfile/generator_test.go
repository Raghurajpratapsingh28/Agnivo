package dockerfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/detect"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/dockerfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomDockerfilePrecedence(t *testing.T) {
	dir := t.TempDir()
	custom := "FROM alpine:3.20\nCMD [\"echo\", \"custom\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(custom), 0o644))

	res, err := dockerfile.NewGenerator().Generate(dir, detect.Framework{Name: "go"})
	require.NoError(t, err)
	assert.False(t, res.Generated)
	assert.Equal(t, "custom", res.Version)
	assert.Equal(t, custom, res.Content)
}

func TestGenerateGoDockerfile(t *testing.T) {
	dir := t.TempDir()
	res, err := dockerfile.NewGenerator().Generate(dir, detect.Framework{Name: "go", Runtime: "go1.22"})
	require.NoError(t, err)
	assert.True(t, res.Generated)
	assert.Contains(t, res.Content, "gcr.io/distroless/static")
	assert.Contains(t, res.Content, "USER nonroot")
}

func TestGenerateNextJSDockerfile(t *testing.T) {
	dir := t.TempDir()
	res, err := dockerfile.NewGenerator().Generate(dir, detect.Framework{Name: "nextjs"})
	require.NoError(t, err)
	assert.True(t, res.Generated)
	assert.True(t, strings.Contains(res.Content, "NODE_ENV=production"))
}
