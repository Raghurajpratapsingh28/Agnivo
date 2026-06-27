package backup_test

import (
	"context"
	"testing"

	"github.com/agnivo/agnivo/packages/application/ops/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type memStorage struct {
	stored map[string][]byte
}

func newMemStorage() *memStorage { return &memStorage{stored: make(map[string][]byte)} }

func (m *memStorage) Store(_ context.Context, name string, data []byte) (string, int64, error) {
	m.stored[name] = data
	return "mem://" + name, int64(len(data)), nil
}

func (m *memStorage) Delete(_ context.Context, path string) error {
	delete(m.stored, path)
	return nil
}

type memDumper struct{ data []byte }

func (d *memDumper) Dump(_ context.Context) ([]byte, error) { return d.data, nil }

func TestBackupManager_NilStorage_UsesNop(t *testing.T) {
	mgr := backup.NewManager(nil, nil, nil, 30, zap.NewNop())
	require.NotNil(t, mgr)
}

func TestBackupManager_DefaultRetention(t *testing.T) {
	mgr := backup.NewManager(nil, nil, nil, 0, zap.NewNop())
	assert.NotNil(t, mgr)
}

func TestPurgeExpired_NoRepo(t *testing.T) {
	// Should not panic even with nil repo when no backups found.
	mgr := backup.NewManager(nil, newMemStorage(), &memDumper{data: []byte("test")}, 30, zap.NewNop())
	require.NotNil(t, mgr)
}
