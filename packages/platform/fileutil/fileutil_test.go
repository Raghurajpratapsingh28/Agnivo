package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAtomicAndExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	require.NoError(t, fileutil.WriteAtomic(path, []byte("hello"), 0o600))

	assert.True(t, fileutil.Exists(path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	// No leftover temp files.
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 1)
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))
	require.NoError(t, fileutil.CopyFile(src, dst, 0o600))
	got, _ := os.ReadFile(dst)
	assert.Equal(t, "data", string(got))
}

func TestSafeJoin(t *testing.T) {
	base := "/srv/app"
	ok, err := fileutil.SafeJoin(base, "logs", "build.txt")
	require.NoError(t, err)
	assert.Equal(t, "/srv/app/logs/build.txt", ok)

	_, err = fileutil.SafeJoin(base, "..", "etc", "passwd")
	require.Error(t, err)
}

func TestIsDir(t *testing.T) {
	dir := t.TempDir()
	assert.True(t, fileutil.IsDir(dir))
	assert.False(t, fileutil.IsDir(filepath.Join(dir, "nope")))
}
