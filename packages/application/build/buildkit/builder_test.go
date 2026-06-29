package buildkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildkit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteBuildContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	assert.NoError(t, buildkit.WriteBuildContext(dir))
	assert.Error(t, buildkit.WriteBuildContext(""))
}

func TestCacheRefs(t *testing.T) {
	cfg := config.Builder{
		Cache: config.BuildCacheConfig{Enabled: true, InlineCache: true},
		ECR:   config.ECRConfig{Enabled: true, Registry: "123.dkr.ecr.us-east-1.amazonaws.com"},
	}
	from, to := buildkit.CacheRefs(cfg, "agnivo/org/app")
	assert.Len(t, from, 1)
	assert.Len(t, to, 1)
	assert.Contains(t, from[0], "buildcache")
}

func TestFormatTags(t *testing.T) {
	s := buildkit.FormatTags([]string{"a:1", "b:2"})
	assert.Contains(t, s, "a:1")
	assert.Contains(t, s, "b:2")
}
