package project

import (
	"encoding/json"
	"time"
)

// Status is the project lifecycle state.
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
	StatusDeleted  Status = "deleted"
)

// Visibility controls project access.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

// Project is the project aggregate root.
type Project struct {
	ID             string          `db:"id" json:"id"`
	OrgID          string          `db:"org_id" json:"org_id"`
	Name           string          `db:"name" json:"name"`
	Slug           string          `db:"slug" json:"slug"`
	Description    string          `db:"description" json:"description"`
	RepoURL        string          `db:"repo_url" json:"repo_url"`
	RepoProvider   string          `db:"repo_provider" json:"repo_provider"`
	Branch         string          `db:"branch" json:"branch"`
	DefaultRuntime string          `db:"default_runtime" json:"default_runtime"`
	Framework      string          `db:"framework" json:"framework"`
	BuildMethod    string          `db:"build_method" json:"build_method"`
	Region         string          `db:"region" json:"region"`
	SleepConfig    json.RawMessage `db:"sleep_config" json:"sleep_config"`
	Visibility     Visibility      `db:"visibility" json:"visibility"`
	Status         Status          `db:"status" json:"status"`
	Labels         json.RawMessage `db:"labels" json:"labels"`
	Tags           []string        `db:"tags" json:"tags"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata"`
	CreatedBy      string          `db:"created_by" json:"created_by"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
	ArchivedAt     *time.Time      `db:"archived_at" json:"archived_at,omitempty"`
	DeletedAt      *time.Time      `db:"deleted_at" json:"deleted_at,omitempty"`
}

// IsLive reports whether the project is usable.
func (p Project) IsLive() bool {
	return p.Status == StatusActive && p.DeletedAt == nil
}
