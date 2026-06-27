package organization

import (
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/strx"
)

// Organization is the tenant aggregate root.
type Organization struct {
	ID             string          `db:"id"`
	Name           string          `db:"name"`
	Slug           string          `db:"slug"`
	AvatarURL      string          `db:"avatar_url"`
	PlanTier       string          `db:"plan_tier"`
	BillingOwnerID *string         `db:"billing_owner_id"`
	Settings       json.RawMessage `db:"settings"`
	Metadata       json.RawMessage `db:"metadata"`
	Limits         json.RawMessage `db:"limits"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`
	DeletedAt      *time.Time      `db:"deleted_at"`
}

// NormalizeSlug slugifies a candidate organization slug.
func NormalizeSlug(s string) string { return strx.Slugify(s) }
