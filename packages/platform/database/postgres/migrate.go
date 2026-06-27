package postgres

import (
	"context"
	"io/fs"
	"sort"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

// Migration is a single forward schema change. Versions must be unique and are
// applied in ascending lexical order, so zero-pad numeric prefixes
// (0001_init.sql, 0002_add_index.sql).
type Migration struct {
	Version string
	SQL     string
}

// migrationsTableDDL creates the bookkeeping table used to track applied
// migrations. It is itself idempotent.
const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version     TEXT PRIMARY KEY,
	applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// LoadMigrations reads all *.sql files from fsys (typically an embed.FS),
// using each file's base name (sans extension) as the version. The result is
// sorted by version.
func LoadMigrations(fsys fs.FS) ([]Migration, error) {
	var migrations []Migration
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		data, rerr := fs.ReadFile(fsys, path)
		if rerr != nil {
			return rerr
		}
		base := path[strings.LastIndex(path, "/")+1:]
		version := strings.TrimSuffix(base, ".sql")
		migrations = append(migrations, Migration{Version: version, SQL: string(data)})
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "postgres: load migrations")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

// Migrate applies any migrations not yet recorded in schema_migrations, each in
// its own transaction, in ascending version order. It is safe to run on every
// boot and concurrently across replicas: a transaction-level advisory lock
// serializes the runner so only one process migrates at a time. Already-applied
// migrations are skipped, making the operation idempotent.
func (db *DB) Migrate(ctx context.Context, migrations []Migration) error {
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })

	// Serialize concurrent migrators with a session advisory lock. The lock is
	// released when the connection returns to the pool.
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "postgres: acquire migrate conn")
	}
	defer conn.Release()

	// 4242 is an arbitrary, application-stable advisory lock key for migrations.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(4242)"); err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "postgres: acquire advisory lock")
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock(4242)") }()

	if _, err := conn.Exec(ctx, migrationsTableDDL); err != nil {
		return errors.Wrap(err, classify(err), "postgres: create migrations table")
	}

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := applyOne(ctx, conn, m); err != nil {
			return err
		}
		db.log.Info("applied migration", zap.String("version", m.Version))
	}
	return nil
}

// applyOne runs a single migration and records it within one transaction so a
// failure leaves neither partial schema nor a bookkeeping row.
func applyOne(ctx context.Context, conn pgxConn, m Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "postgres: begin migration "+m.Version)
	}
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		_ = tx.Rollback(ctx)
		return errors.Wrap(err, classify(err), "postgres: apply migration "+m.Version)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.Version); err != nil {
		_ = tx.Rollback(ctx)
		return errors.Wrap(err, classify(err), "postgres: record migration "+m.Version)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrap(err, classify(err), "postgres: commit migration "+m.Version)
	}
	return nil
}

func loadApplied(ctx context.Context, conn pgxConn) (map[string]struct{}, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, errors.Wrap(err, classify(err), "postgres: query applied migrations")
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, errors.Wrap(err, errors.CodeInternal, "postgres: scan migration version")
		}
		applied[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, classify(err), "postgres: iterate migrations")
	}
	return applied, nil
}

// pgxConn is the subset of *pgxpool.Conn used by the migration runner, declared
// as an interface for testability.
type pgxConn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}
