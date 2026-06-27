package user

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
)

// CreateEmailToken stores an email verification token hash.
func (r *Repository) CreateEmailToken(ctx context.Context, userID, hash string, expiresAt time.Time) error {
	const q = `INSERT INTO identity_email_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,now())`
	_, err := r.db.Conn(ctx).Exec(ctx, q, idx.NewUUID(), userID, hash, expiresAt)
	return postgres.Translate(err, "user: create email token")
}

// ConsumeEmailToken marks a token used and returns the user ID.
func (r *Repository) ConsumeEmailToken(ctx context.Context, hash string) (string, error) {
	const q = `UPDATE identity_email_tokens SET used_at=now()
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`
	var userID string
	row := r.db.Conn(ctx).QueryRow(ctx, q, hash)
	if err := row.Scan(&userID); err != nil {
		return "", postgres.Translate(err, "user: consume email token")
	}
	return userID, nil
}

// CreatePasswordResetToken stores a password reset token hash.
func (r *Repository) CreatePasswordResetToken(ctx context.Context, userID, hash string, expiresAt time.Time) error {
	const q = `INSERT INTO identity_password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,now())`
	_, err := r.db.Conn(ctx).Exec(ctx, q, idx.NewUUID(), userID, hash, expiresAt)
	return postgres.Translate(err, "user: create password reset token")
}

// ConsumePasswordResetToken marks a reset token used and returns the user ID.
func (r *Repository) ConsumePasswordResetToken(ctx context.Context, hash string) (string, error) {
	const q = `UPDATE identity_password_reset_tokens SET used_at=now()
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`
	var userID string
	row := r.db.Conn(ctx).QueryRow(ctx, q, hash)
	if err := row.Scan(&userID); err != nil {
		return "", errors.Unauthenticated("invalid or expired reset token")
	}
	return userID, nil
}
