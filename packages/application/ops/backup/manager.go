// Package backup implements automated backup scheduling, execution, verification,
// retention enforcement, and disaster-recovery metadata.
package backup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"go.uber.org/zap"
)

// BackupStorage abstracts the storage layer for backup artifacts.
type BackupStorage interface {
	// Store persists data and returns the storage path and size in bytes.
	Store(ctx context.Context, name string, data []byte) (path string, sizeBytes int64, err error)
	// Delete removes a stored artifact.
	Delete(ctx context.Context, path string) error
}

// DatabaseDumper produces a serialized snapshot of the database.
type DatabaseDumper interface {
	Dump(ctx context.Context) (data []byte, err error)
}

// nopStorage is a no-op backend for environments without real object storage.
type nopStorage struct{}

func (nopStorage) Store(_ context.Context, name string, _ []byte) (string, int64, error) {
	return "nop://" + name, 0, nil
}
func (nopStorage) Delete(_ context.Context, _ string) error { return nil }

// Manager orchestrates the backup lifecycle.
type Manager struct {
	repo          *store.Repository
	storage       BackupStorage
	dumper        DatabaseDumper
	retentionDays int
	log           *zap.Logger
}

// NewManager constructs a backup Manager.
func NewManager(repo *store.Repository, storage BackupStorage, dumper DatabaseDumper, retentionDays int, log *zap.Logger) *Manager {
	if storage == nil {
		storage = nopStorage{}
	}
	if retentionDays <= 0 {
		retentionDays = 30
	}
	return &Manager{
		repo:          repo,
		storage:       storage,
		dumper:        dumper,
		retentionDays: retentionDays,
		log:           log,
	}
}

// RunDatabaseBackup executes a full database backup.
func (m *Manager) RunDatabaseBackup(ctx context.Context, correlationID string) (model.Backup, error) {
	if correlationID == "" {
		correlationID = idx.NewUUID()
	}

	backup, err := m.repo.InsertBackup(ctx, model.Backup{
		Kind:          model.BackupDatabase,
		Status:        model.BackupRunning,
		RetentionDays: m.retentionDays,
		CorrelationID: correlationID,
	})
	if err != nil {
		return model.Backup{}, err
	}

	start := time.Now()
	m.log.Info("backup: database backup started", zap.String("id", backup.ID))

	var data []byte
	if m.dumper != nil {
		data, err = m.dumper.Dump(ctx)
		if err != nil {
			_ = m.repo.SetBackupStatus(ctx, backup.ID, model.BackupFailed, err.Error())
			return backup, errors.Wrapf(err, errors.CodeInternal, "backup: dump failed")
		}
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256(data))
	name := fmt.Sprintf("db-%s-%s.dump", time.Now().UTC().Format("20060102-150405"), backup.ID[:8])

	path, sizeBytes, err := m.storage.Store(ctx, name, data)
	if err != nil {
		_ = m.repo.SetBackupStatus(ctx, backup.ID, model.BackupFailed, err.Error())
		return backup, errors.Wrapf(err, errors.CodeInternal, "backup: store failed")
	}

	durationSecs := int64(time.Since(start).Seconds())
	expiresAt := time.Now().UTC().AddDate(0, 0, m.retentionDays)
	backup.ExpiresAt = &expiresAt

	if err := m.repo.CompleteBackup(ctx, backup.ID, path, checksum, sizeBytes, durationSecs); err != nil {
		return backup, err
	}

	m.log.Info("backup: completed",
		zap.String("id", backup.ID),
		zap.String("path", path),
		zap.Int64("bytes", sizeBytes),
		zap.Int64("duration_secs", durationSecs))

	backup.Status = model.BackupCompleted
	backup.StoragePath = path
	backup.Checksum = checksum
	backup.SizeBytes = sizeBytes
	return backup, nil
}

// PurgeExpired deletes backups that have passed their retention window.
func (m *Manager) PurgeExpired(ctx context.Context) (int, error) {
	backups, err := m.repo.ListExpiredBackups(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, b := range backups {
		if err := m.storage.Delete(ctx, b.StoragePath); err != nil {
			m.log.Warn("backup: delete storage failed",
				zap.String("id", b.ID),
				zap.String("path", b.StoragePath),
				zap.Error(err))
		}
		if err := m.repo.SetBackupStatus(ctx, b.ID, model.BackupFailed, "purged"); err != nil {
			m.log.Warn("backup: mark purged failed", zap.String("id", b.ID), zap.Error(err))
			continue
		}
		removed++
	}
	if removed > 0 {
		m.log.Info("backup: purged expired", zap.Int("count", removed))
	}
	return removed, nil
}
