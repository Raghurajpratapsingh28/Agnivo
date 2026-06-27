package user

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/repository"
	"github.com/jackc/pgx/v5"
)

// Repository persists users.
type Repository struct {
	repo *repository.Repository[User]
	db   *postgres.DB
}

// NewRepository constructs a user repository.
func NewRepository(db *postgres.DB) *Repository {
	return &Repository{
		repo: repository.New[User](db, "identity_users",
			repository.WithSoftDelete("deleted_at"),
			repository.WithColumns(
				"id", "email", "email_verified_at", "password_hash", "display_name", "avatar_url",
				"timezone", "locale", "preferences", "status", "metadata",
				"last_login_at", "last_active_at", "created_at", "updated_at", "deleted_at",
			),
		),
		db: db,
	}
}

// Create inserts a new user.
func (r *Repository) Create(ctx context.Context, email, passwordHash, displayName string) (User, error) {
	prefs, _ := json.Marshal(map[string]any{})
	meta, _ := json.Marshal(map[string]any{})
	now := time.Now().UTC()
	u, err := r.repo.Insert(ctx, map[string]any{
		"id": idx.NewUUID(), "email": strings.ToLower(strings.TrimSpace(email)),
		"password_hash": passwordHash, "display_name": displayName,
		"preferences": prefs, "metadata": meta, "status": StatusPending,
		"created_at": now, "updated_at": now,
	})
	if err != nil {
		return User{}, postgres.Translate(err, "user: create")
	}
	return u, nil
}

// GetByID fetches a live user by ID.
func (r *Repository) GetByID(ctx context.Context, id string) (User, error) {
	u, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return User{}, postgres.Translate(err, "user: get by id")
	}
	return u, nil
}

// GetByEmail fetches a live user by email (case-insensitive).
func (r *Repository) GetByEmail(ctx context.Context, email string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	const q = `SELECT id, email, email_verified_at, password_hash, display_name, avatar_url,
		timezone, locale, preferences, status, metadata, last_login_at, last_active_at,
		created_at, updated_at, deleted_at
		FROM identity_users WHERE lower(email) = $1 AND deleted_at IS NULL LIMIT 1`
	rows, err := r.db.Conn(ctx).Query(ctx, q, email)
	if err != nil {
		return User{}, postgres.Translate(err, "user: get by email")
	}
	u, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[User])
	if err != nil {
		return User{}, postgres.Translate(err, "user: get by email")
	}
	return u, nil
}

// UpdateProfile updates mutable profile fields.
func (r *Repository) UpdateProfile(ctx context.Context, id string, displayName, avatarURL, timezone, locale string, preferences json.RawMessage) (User, error) {
	vals := map[string]any{"updated_at": time.Now().UTC()}
	if displayName != "" {
		vals["display_name"] = displayName
	}
	if avatarURL != "" {
		vals["avatar_url"] = avatarURL
	}
	if timezone != "" {
		vals["timezone"] = timezone
	}
	if locale != "" {
		vals["locale"] = locale
	}
	if len(preferences) > 0 {
		vals["preferences"] = preferences
	}
	u, err := r.repo.Update(ctx, id, vals)
	if err != nil {
		return User{}, postgres.Translate(err, "user: update profile")
	}
	return u, nil
}

// SetPassword updates the password hash.
func (r *Repository) SetPassword(ctx context.Context, id, hash string) error {
	_, err := r.repo.Update(ctx, id, map[string]any{"password_hash": hash, "updated_at": time.Now().UTC()})
	return postgres.Translate(err, "user: set password")
}

// Activate marks the user active and verified.
func (r *Repository) Activate(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.repo.Update(ctx, id, map[string]any{
		"status": StatusActive, "email_verified_at": now, "updated_at": now,
	})
	return postgres.Translate(err, "user: activate")
}

// Suspend suspends the account.
func (r *Repository) Suspend(ctx context.Context, id string) error {
	_, err := r.repo.Update(ctx, id, map[string]any{"status": StatusSuspended, "updated_at": time.Now().UTC()})
	return postgres.Translate(err, "user: suspend")
}

// SoftDelete soft-deletes the account.
func (r *Repository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.repo.SoftDelete(ctx, id)
	if err != nil {
		return postgres.Translate(err, "user: delete")
	}
	_, err = r.repo.Update(ctx, id, map[string]any{"status": StatusDeleted, "updated_at": time.Now().UTC()})
	return postgres.Translate(err, "user: delete")
}

// RecordLogin updates last login timestamps.
func (r *Repository) RecordLogin(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.repo.Update(ctx, id, map[string]any{
		"last_login_at": now, "last_active_at": now, "updated_at": now,
	})
	return postgres.Translate(err, "user: record login")
}

// ExistsEmail reports whether a live user with email exists (for registration).
func (r *Repository) ExistsEmail(ctx context.Context, email string) (bool, error) {
	return r.repo.Exists(ctx, repository.Eq("email", strings.ToLower(strings.TrimSpace(email))))
}

// ErrNotFound is returned when a user is not found.
var ErrNotFound = errors.NotFound("user not found")
