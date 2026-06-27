package apikey

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/identity/tokencrypto"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// APIKey is an organization-scoped API key.
type APIKey struct {
	ID          string     `db:"id" json:"id"`
	OrgID       string     `db:"org_id" json:"org_id"`
	Name        string     `db:"name" json:"name"`
	Prefix      string     `db:"prefix" json:"prefix"`
	KeyHash     string     `db:"-" json:"-"`
	Scopes      []string   `db:"scopes" json:"scopes"`
	ExpiresAt   *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	DisabledAt  *time.Time `db:"disabled_at" json:"disabled_at,omitempty"`
	LastUsedAt  *time.Time `db:"last_used_at" json:"last_used_at,omitempty"`
	LastUsedIP  string     `db:"last_used_ip" json:"last_used_ip,omitempty"`
	CreatedBy   string     `db:"created_by" json:"created_by"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

// CreateResult holds the one-time secret shown to the caller.
type CreateResult struct {
	Key    APIKey `json:"key"`
	Secret string `json:"secret"`
}

// Repository persists API keys.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an API key repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Create inserts a new API key and returns the one-time secret.
func (r *Repository) Create(ctx context.Context, orgID, name string, scopes []string, expiresAt *time.Time, createdBy string) (CreateResult, error) {
	prefix := tokencrypto.APIKeyPrefix()
	secret, hash := tokencrypto.Generate(32)
	now := time.Now().UTC()
	const q = `INSERT INTO identity_api_keys
		(id, org_id, name, prefix, key_hash, scopes, expires_at, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		RETURNING id, org_id, name, prefix, scopes, expires_at, disabled_at, last_used_at, last_used_ip, created_by, created_at, updated_at`
	var k APIKey
	row := r.db.Conn(ctx).QueryRow(ctx, q, idx.NewUUID(), orgID, name, prefix, hash, scopes, expiresAt, createdBy, now)
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.Scopes, &k.ExpiresAt,
		&k.DisabledAt, &k.LastUsedAt, &k.LastUsedIP, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return CreateResult{}, postgres.Translate(err, "apikey: create")
	}
	return CreateResult{Key: k, Secret: tokencrypto.FormatAPIKey(prefix, secret)}, nil
}

// GetByPrefix finds a live key by prefix for authentication.
func (r *Repository) GetByPrefix(ctx context.Context, prefix string) (APIKey, error) {
	const q = `SELECT id, org_id, name, prefix, key_hash, scopes, expires_at, disabled_at, last_used_at, last_used_ip, created_by, created_at, updated_at
		FROM identity_api_keys WHERE prefix=$1 AND disabled_at IS NULL
		AND (expires_at IS NULL OR expires_at > now()) LIMIT 1`
	var k APIKey
	row := r.db.Conn(ctx).QueryRow(ctx, q, prefix)
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.KeyHash, &k.Scopes, &k.ExpiresAt,
		&k.DisabledAt, &k.LastUsedAt, &k.LastUsedIP, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return APIKey{}, postgres.Translate(err, "apikey: get")
	}
	return k, nil
}

// ListByOrg returns API keys for an organization (without secrets).
func (r *Repository) ListByOrg(ctx context.Context, orgID string) ([]APIKey, error) {
	const q = `SELECT id, org_id, name, prefix, scopes, expires_at, disabled_at, last_used_at, last_used_ip, created_by, created_at, updated_at
		FROM identity_api_keys WHERE org_id=$1 ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, postgres.Translate(err, "apikey: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[APIKey])
}

// Rotate replaces the key hash and returns a new secret.
func (r *Repository) Rotate(ctx context.Context, orgID, keyID string) (CreateResult, error) {
	secret, hash := tokencrypto.Generate(32)
	const q = `UPDATE identity_api_keys SET key_hash=$3, updated_at=now()
		WHERE id=$1 AND org_id=$2 AND disabled_at IS NULL
		RETURNING id, org_id, name, prefix, scopes, expires_at, disabled_at, last_used_at, last_used_ip, created_by, created_at, updated_at`
	var k APIKey
	row := r.db.Conn(ctx).QueryRow(ctx, q, keyID, orgID, hash)
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.Scopes, &k.ExpiresAt,
		&k.DisabledAt, &k.LastUsedAt, &k.LastUsedIP, &k.CreatedBy, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return CreateResult{}, postgres.Translate(err, "apikey: rotate")
	}
	return CreateResult{Key: k, Secret: tokencrypto.FormatAPIKey(k.Prefix, secret)}, nil
}

// Disable marks a key disabled.
func (r *Repository) Disable(ctx context.Context, orgID, keyID string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE identity_api_keys SET disabled_at=now(), updated_at=now() WHERE id=$1 AND org_id=$2 AND disabled_at IS NULL`,
		keyID, orgID)
	if err != nil {
		return postgres.Translate(err, "apikey: disable")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("api key not found")
	}
	return nil
}

// Delete permanently removes a key.
func (r *Repository) Delete(ctx context.Context, orgID, keyID string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`DELETE FROM identity_api_keys WHERE id=$1 AND org_id=$2`, keyID, orgID)
	if err != nil {
		return postgres.Translate(err, "apikey: delete")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("api key not found")
	}
	return nil
}

// RecordUsage updates last_used_at and IP.
func (r *Repository) RecordUsage(ctx context.Context, keyID, ip string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE identity_api_keys SET last_used_at=now(), last_used_ip=$2, updated_at=now() WHERE id=$1`, keyID, ip)
	return postgres.Translate(err, "apikey: record usage")
}

// VerifySecret compares the provided secret against the stored hash.
func VerifySecret(k APIKey, secret string) bool {
	return tokencrypto.ConstantTimeEqual(k.KeyHash, tokencrypto.Hash(secret))
}
