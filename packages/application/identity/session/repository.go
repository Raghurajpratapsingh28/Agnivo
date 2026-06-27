package session

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// DeviceType classifies the client.
type DeviceType string

const (
	DeviceBrowser DeviceType = "browser"
	DeviceCLI     DeviceType = "cli"
	DeviceAPI     DeviceType = "api"
)

// Session is a persisted refresh session.
type Session struct {
	ID               string     `db:"id" json:"id"`
	UserID           string     `db:"user_id" json:"user_id"`
	OrgID            *string    `db:"org_id" json:"org_id,omitempty"`
	RefreshTokenHash string     `db:"refresh_token_hash" json:"-"`
	DeviceName       string     `db:"device_name" json:"device_name"`
	DeviceType       DeviceType `db:"device_type" json:"device_type"`
	IPAddress        string     `db:"ip_address" json:"ip_address"`
	UserAgent        string     `db:"user_agent" json:"user_agent"`
	RememberMe       bool       `db:"remember_me" json:"remember_me"`
	ExpiresAt        time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt        *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	LastUsedAt       *time.Time `db:"last_used_at" json:"last_used_at,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
}

// Repository persists sessions.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a session repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Create inserts a new session.
func (r *Repository) Create(ctx context.Context, userID string, orgID *string, refreshHash, deviceName string, deviceType DeviceType, ip, ua string, remember bool, expiresAt time.Time) (Session, error) {
	const q = `INSERT INTO identity_sessions
		(id, user_id, org_id, refresh_token_hash, device_name, device_type, ip_address, user_agent, remember_me, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
		RETURNING id, user_id, org_id, refresh_token_hash, device_name, device_type, ip_address, user_agent, remember_me, expires_at, revoked_at, last_used_at, created_at`
	id := idx.NewUUID()
	row := r.db.Conn(ctx).QueryRow(ctx, q, id, userID, orgID, refreshHash, deviceName, deviceType, ip, ua, remember, expiresAt)
	return scan(row)
}

// GetByRefreshHash finds a live session by refresh token hash.
func (r *Repository) GetByRefreshHash(ctx context.Context, hash string) (Session, error) {
	const q = `SELECT id, user_id, org_id, refresh_token_hash, device_name, device_type, ip_address, user_agent, remember_me, expires_at, revoked_at, last_used_at, created_at
		FROM identity_sessions WHERE refresh_token_hash=$1 AND revoked_at IS NULL AND expires_at > now() LIMIT 1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, hash)
	s, err := scan(row)
	if err != nil {
		return Session{}, postgres.Translate(err, "session: get")
	}
	return s, nil
}

// RotateRefresh updates the refresh token hash (rotation) and last_used.
func (r *Repository) RotateRefresh(ctx context.Context, sessionID, newHash string) error {
	const q = `UPDATE identity_sessions SET refresh_token_hash=$2, last_used_at=now() WHERE id=$1 AND revoked_at IS NULL`
	tag, err := r.db.Conn(ctx).Exec(ctx, q, sessionID, newHash)
	if err != nil {
		return postgres.Translate(err, "session: rotate")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("session not found")
	}
	return nil
}

// Revoke revokes a single session.
func (r *Repository) Revoke(ctx context.Context, sessionID, userID string) error {
	const q = `UPDATE identity_sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`
	tag, err := r.db.Conn(ctx).Exec(ctx, q, sessionID, userID)
	if err != nil {
		return postgres.Translate(err, "session: revoke")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("session not found")
	}
	return nil
}

// RevokeAll revokes all sessions for a user except optionally one.
func (r *Repository) RevokeAll(ctx context.Context, userID, exceptID string) (int64, error) {
	const q = `UPDATE identity_sessions SET revoked_at=now()
		WHERE user_id=$1 AND revoked_at IS NULL AND ($2 = '' OR id <> $2)`
	tag, err := r.db.Conn(ctx).Exec(ctx, q, userID, exceptID)
	if err != nil {
		return 0, postgres.Translate(err, "session: revoke all")
	}
	return tag.RowsAffected(), nil
}

// ListByUser returns active sessions for a user.
func (r *Repository) ListByUser(ctx context.Context, userID string) ([]Session, error) {
	const q = `SELECT id, user_id, org_id, refresh_token_hash, device_name, device_type, ip_address, user_agent, remember_me, expires_at, revoked_at, last_used_at, created_at
		FROM identity_sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > now() ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, postgres.Translate(err, "session: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Session])
}

func scan(row pgx.Row) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshTokenHash, &s.DeviceName, &s.DeviceType,
		&s.IPAddress, &s.UserAgent, &s.RememberMe, &s.ExpiresAt, &s.RevokedAt, &s.LastUsedAt, &s.CreatedAt)
	return s, err
}
