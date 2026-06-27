package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type widget struct {
	ID      string     `db:"id"`
	Name    string     `db:"name"`
	Version int64      `db:"version"`
	Deleted *time.Time `db:"deleted_at"`
}

const widgetDDL = `
CREATE TABLE IF NOT EXISTS widgets (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	version    BIGINT NOT NULL DEFAULT 0,
	deleted_at TIMESTAMPTZ
)`

func setup(t *testing.T) (*postgres.DB, *repository.Repository[widget]) {
	t.Helper()
	url := os.Getenv("DATABASE_TEST_URL")
	if url == "" {
		t.Skip("DATABASE_TEST_URL not set; skipping repository integration test")
	}
	cfg := &config.Config{Database: config.Database{Enabled: true, URL: url, MaxConns: 5, QueryTimeout: 5 * time.Second}}
	db, err := postgres.New(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(db.Close)

	_, err = db.Exec(context.Background(), widgetDDL)
	require.NoError(t, err)
	_, _ = db.Exec(context.Background(), "TRUNCATE widgets")

	repo := repository.New[widget](db, "widgets",
		repository.WithSoftDelete("deleted_at"),
		repository.WithOptimisticLock("version"))
	return db, repo
}

func TestCRUD(t *testing.T) {
	_, repo := setup(t)
	ctx := context.Background()

	created, err := repo.Insert(ctx, map[string]any{"id": "w1", "name": "alpha"})
	require.NoError(t, err)
	assert.Equal(t, "alpha", created.Name)

	got, err := repo.GetByID(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.ID)

	updated, err := repo.Update(ctx, "w1", map[string]any{"name": "beta"})
	require.NoError(t, err)
	assert.Equal(t, "beta", updated.Name)

	_, err = repo.GetByID(ctx, "missing")
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
}

func TestOptimisticLock(t *testing.T) {
	_, repo := setup(t)
	ctx := context.Background()
	_, err := repo.Insert(ctx, map[string]any{"id": "w2", "name": "x"})
	require.NoError(t, err)

	updated, err := repo.UpdateOptimistic(ctx, "w2", 0, map[string]any{"name": "y"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), updated.Version)

	// Stale version is rejected.
	_, err = repo.UpdateOptimistic(ctx, "w2", 0, map[string]any{"name": "z"})
	assert.True(t, errors.Is(err, repository.ErrOptimisticLock))
}

func TestSoftDeleteHidesRows(t *testing.T) {
	_, repo := setup(t)
	ctx := context.Background()
	_, err := repo.Insert(ctx, map[string]any{"id": "w3", "name": "s"})
	require.NoError(t, err)

	ok, err := repo.SoftDelete(ctx, "w3")
	require.NoError(t, err)
	assert.True(t, ok)

	_, err = repo.GetByID(ctx, "w3")
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
}

func TestPaginateAndBulk(t *testing.T) {
	_, repo := setup(t)
	ctx := context.Background()

	cols := []string{"id", "name"}
	rows := make([][]any, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, []any{"bulk-" + string(rune('a'+i)), "n"})
	}
	n, err := repo.BulkInsert(ctx, cols, rows)
	require.NoError(t, err)
	assert.Equal(t, int64(25), n)

	page, err := repo.Paginate(ctx, repository.Eq("name", "n"),
		repository.PageParams{Page: 1, PageSize: 10, OrderBy: "id"})
	require.NoError(t, err)
	assert.Equal(t, int64(25), page.Total)
	assert.Len(t, page.Items, 10)
	assert.Equal(t, int64(3), page.TotalPages)

	cp, err := repo.PaginateCursor(ctx, repository.Eq("name", "n"),
		repository.CursorParams{Column: "id", Limit: 10}, func(w widget) string { return w.ID })
	require.NoError(t, err)
	assert.Len(t, cp.Items, 10)
	assert.True(t, cp.HasMore)
	assert.NotEmpty(t, cp.NextCursor)
}
