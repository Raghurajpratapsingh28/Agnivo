package detect_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/detect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectNextJS(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
		"dependencies": {"next": "14.0.0", "react": "18.0.0"}
	}`), 0o644))

	fw := detect.NewDetector().Detect(dir)
	assert.Equal(t, "nextjs", fw.Name)
	assert.Equal(t, "node20", fw.Runtime)
}

func TestDetectFastAPI(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("fastapi\nuvicorn\n"), 0o644))

	fw := detect.NewDetector().Detect(dir)
	assert.Equal(t, "fastapi", fw.Name)
}

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\ngo 1.22\n"), 0o644))

	fw := detect.NewDetector().Detect(dir)
	assert.Equal(t, "go", fw.Name)
}

func TestDetectCustomDockerfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"4"}}`), 0o644))

	fw := detect.NewDetector().Detect(dir)
	assert.Equal(t, "dockerfile", fw.Name)
}

func TestDetectStatic(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644))

	fw := detect.NewDetector().Detect(dir)
	assert.Equal(t, "static", fw.Name)
}
