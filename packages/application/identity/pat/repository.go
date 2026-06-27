package pat

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/application/identity/tokencrypto"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Token is a personal access token.
type Token struct {
	ID         string          `db:"id" json:"id"`
	UserID     string          `db:"user_id" json:"user_id"`
	OrgID      *string         `db:"org_id" json:"org_id,omitempty"`
	Name       string          `db:"name" json:"name"`
	Prefix     string          `db:"prefix" json:"prefix"`
	TokenHash  string          `db:"-" json:"-"`
	Scopes     []string        `db:"scopes" json:"scopes"`
	ExpiresAt  *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	RevokedAt  *time.Time      `db:"revoked_at" json:"revoked_at,omitempty"`
	LastUsedAt *time.Time      `db:"last_used_at" json:"last_used_at,omitempty"`
	Metadata   json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
}

// CreateResult holds the one-time secret.
type CreateResult struct {
	Token  Token  `json:"token"`
	Secret string `json:"secret"`
}

// Repository persists personal access tokens.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a PAT repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Create inserts a new PAT.
func (r *Repository) Create(ctx context.Context, userID string, orgID *string, name string, scopes []string, expiresAt *time.Time, metadata json.RawMessage) (CreateResult, error) {
	prefix := "pat_" + idx.Hex(4)
	secret, hash := tokencrypto.Generate(32)
	if metadata == nil {
		metadata, _ = json.Marshal(map[string]any{})
	}
	now := time.Now().UTC()
	const q = `INSERT INTO identity_personal_access_tokens
		(id, user_id, org_id, name, prefix, token_hash, scopes, expires_at, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		RETURNING id, user_id, org_id, name, prefix, scopes, expires_at, revoked_at, last_used_at, metadata, created_at, updated_at`
	var t Token
	row := r.db.Conn(ctx).QueryRow(ctx, q, idx.NewUUID(), userID, orgID, name, prefix, hash, scopes, expiresAt, metadata, now)
	err := row.Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt,
		&t.RevokedAt, &t.LastUsedAt, &t.Metadata, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return CreateResult{}, postgres.Translate(err, "pat: create")
	}
	return CreateResult{Token: t, Secret: prefix + "_" + secret}, nil
}

// GetByPrefix finds a live PAT by prefix.
func (r *Repository) GetByPrefix(ctx context.Context, prefix string) (Token, error) {
	const q = `SELECT id, user_id, org_id, name, prefix, token_hash, scopes, expires_at, revoked_at, last_used_at, metadata, created_at, updated_at
		FROM identity_personal_access_tokens WHERE prefix=$1 AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at > now()) LIMIT 1`
	var t Token
	row := r.db.Conn(ctx).QueryRow(ctx, q, prefix)
	err := row.Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.Prefix, &t.TokenHash, &t.Scopes, &t.ExpiresAt,
		&t.RevokedAt, &t.LastUsedAt, &t.Metadata, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return Token{}, postgres.Translate(err, "pat: get")
	}
	return t, nil
}

// ListByUser returns PATs for a user.
func (r *Repository) ListByUser(ctx context.Context, userID string) ([]Token, error) {
	const q = `SELECT id, user_id, org_id, name, prefix, scopes, expires_at, revoked_at, last_used_at, metadata, created_at, updated_at
		FROM identity_personal_access_tokens WHERE user_id=$1 ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, postgres.Translate(err, "pat: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Token])
}

// Revoke marks a PAT revoked.
func (r *Repository) Revoke(ctx context.Context, userID, tokenID string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE identity_personal_access_tokens SET revoked_at=now(), updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, tokenID, userID)
	if err != nil {
		return postgres.Translate(err, "pat: revoke")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("token not found")
	}
	return nil
}

// Rotate replaces the token hash.
func (r *Repository) Rotate(ctx context.Context, userID, tokenID string) (CreateResult, error) {
	secret, hash := tokencrypto.Generate(32)
	const q = `UPDATE identity_personal_access_tokens SET token_hash=$3, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL
		RETURNING id, user_id, org_id, name, prefix, scopes, expires_at, revoked_at, last_used_at, metadata, created_at, updated_at`
	var t Token
	row := r.db.Conn(ctx).QueryRow(ctx, q, tokenID, userID, hash)
	err := row.Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt,
		&t.RevokedAt, &t.LastUsedAt, &t.Metadata, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return CreateResult{}, postgres.Translate(err, "pat: rotate")
	}
	return CreateResult{Token: t, Secret: t.Prefix + "_" + secret}, nil
}

// RecordUsage updates last_used_at.
func (r *Repository) RecordUsage(ctx context.Context, tokenID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE identity_personal_access_tokens SET last_used_at=now(), updated_at=now() WHERE id=$1`, tokenID)
	return postgres.Translate(err, "pat: record usage")
}

// VerifySecret compares secret against stored hash.
func VerifySecret(t Token, secret string) bool {
	return tokencrypto.ConstantTimeEqual(t.TokenHash, tokencrypto.Hash(secret))
}
