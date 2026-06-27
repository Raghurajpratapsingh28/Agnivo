package postgres_test

import (
	stderrors "errors"
	"testing"
	"testing/fstest"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateNoRows(t *testing.T) {
	err := postgres.Translate(pgx.ErrNoRows, "get user")
	assert.Equal(t, errors.CodeNotFound, errors.CodeOf(err))
	assert.True(t, postgres.IsNotFound(err))
}

func TestTranslateUniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	err := postgres.Translate(pgErr, "insert user")
	assert.Equal(t, errors.CodeAlreadyExists, errors.CodeOf(err))
	assert.True(t, postgres.IsUniqueViolation(err))
	assert.True(t, stderrors.Is(err, pgErr))
}

func TestTranslateSerializationIsRetryable(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "40001"}
	err := postgres.Translate(pgErr, "tx")
	assert.Equal(t, errors.CodeConflict, errors.CodeOf(err))
	assert.True(t, errors.IsRetryable(err))
}

func TestTranslateForeignKey(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503"}
	err := postgres.Translate(pgErr, "insert")
	assert.Equal(t, errors.CodeFailedPrecond, errors.CodeOf(err))
	assert.False(t, errors.IsRetryable(err))
}

func TestTranslateNil(t *testing.T) {
	assert.Nil(t, postgres.Translate(nil, "noop"))
}

func TestLoadMigrationsSorted(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_add_index.sql": {Data: []byte("CREATE INDEX ...")},
		"0001_init.sql":      {Data: []byte("CREATE TABLE ...")},
		"README.md":          {Data: []byte("ignore me")},
		"sub/0003_more.sql":  {Data: []byte("ALTER TABLE ...")},
	}
	migrations, err := postgres.LoadMigrations(fsys)
	require.NoError(t, err)
	require.Len(t, migrations, 3)
	assert.Equal(t, "0001_init", migrations[0].Version)
	assert.Equal(t, "0002_add_index", migrations[1].Version)
	assert.Equal(t, "0003_more", migrations[2].Version)
}

func TestMetricsObserveNilSafe(t *testing.T) {
	var m *postgres.Metrics
	assert.NotPanics(t, func() {
		m.ObserveQuery("select", 0.01, nil)
		m.ObserveTx("commit")
	})
}

func TestMetricsCollectors(t *testing.T) {
	m := postgres.NewMetrics("test")
	assert.NotEmpty(t, m.Collectors())
	assert.NotPanics(t, func() { m.ObserveQuery("select", 0.01, pgx.ErrNoRows) })
}
