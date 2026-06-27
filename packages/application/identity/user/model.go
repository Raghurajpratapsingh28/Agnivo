package user

import (
	"encoding/json"
	"time"
)

// Status is the account lifecycle state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

// User is the user aggregate root.
type User struct {
	ID              string          `db:"id"`
	Email           string          `db:"email"`
	EmailVerifiedAt *time.Time      `db:"email_verified_at"`
	PasswordHash    string          `db:"password_hash"`
	DisplayName     string          `db:"display_name"`
	AvatarURL       string          `db:"avatar_url"`
	Timezone        string          `db:"timezone"`
	Locale          string          `db:"locale"`
	Preferences     json.RawMessage `db:"preferences"`
	Status          Status          `db:"status"`
	Metadata        json.RawMessage `db:"metadata"`
	LastLoginAt     *time.Time      `db:"last_login_at"`
	LastActiveAt    *time.Time      `db:"last_active_at"`
	CreatedAt       time.Time       `db:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
	DeletedAt       *time.Time      `db:"deleted_at"`
}

// IsActive reports whether the user can authenticate.
func (u User) IsActive() bool {
	return u.Status == StatusActive && u.DeletedAt == nil
}

// CanLogin reports whether login should proceed (active or pending with verification).
func (u User) CanLogin() bool {
	return (u.Status == StatusActive || u.Status == StatusPending) && u.DeletedAt == nil
}
