package testkit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/stretchr/testify/require"
)

// ConnectPostgres opens a real PostgreSQL pool when DATABASE_TEST_URL is set,
// skipping the test otherwise. The pool is closed automatically on cleanup.
func ConnectPostgres(t testing.TB) *postgres.DB {
	t.Helper()
	url := os.Getenv("DATABASE_TEST_URL")
	if url == "" {
		t.Skip("DATABASE_TEST_URL not set")
	}
	cfg := &config.Config{
		Database: config.Database{
			Enabled:      true,
			URL:          url,
			MaxConns:     5,
			QueryTimeout: 5 * time.Second,
		},
	}
	db, err := postgres.New(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	return db
}

// ConnectRedis opens a real Redis client when REDIS_TEST_URL is set, skipping
// the test otherwise.
func ConnectRedis(t testing.TB) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("REDIS_TEST_URL not set")
	}
	cfg := &config.Config{
		Redis: config.Redis{Enabled: true, URL: url, PoolSize: 5},
	}
	c, err := redis.New(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// RunMigrations applies SQL migrations via the postgres migration runner.
func RunMigrations(t testing.TB, db *postgres.DB, migrations ...postgres.Migration) {
	t.Helper()
	require.NoError(t, db.Migrate(context.Background(), migrations))
}
