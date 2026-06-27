package organization

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/repository"
	"github.com/jackc/pgx/v5"
)

// Repository persists organizations.
type Repository struct {
	repo *repository.Repository[Organization]
	db   *postgres.DB
}

// NewRepository constructs an organization repository.
func NewRepository(db *postgres.DB) *Repository {
	return &Repository{
		db: db,
		repo: repository.New[Organization](db, "identity_organizations",
			repository.WithSoftDelete("deleted_at"),
			repository.WithColumns(
				"id", "name", "slug", "avatar_url", "plan_tier", "billing_owner_id",
				"settings", "metadata", "limits", "created_at", "updated_at", "deleted_at",
			),
		),
	}
}

// Create inserts a new organization.
func (r *Repository) Create(ctx context.Context, name, slug string, ownerID string) (Organization, error) {
	settings, _ := json.Marshal(map[string]any{})
	meta, _ := json.Marshal(map[string]any{})
	limits, _ := json.Marshal(map[string]any{})
	now := time.Now().UTC()
	o, err := r.repo.Insert(ctx, map[string]any{
		"id": idx.NewUUID(), "name": name, "slug": slug,
		"billing_owner_id": ownerID, "settings": settings, "metadata": meta, "limits": limits,
		"created_at": now, "updated_at": now,
	})
	if err != nil {
		return Organization{}, postgres.Translate(err, "org: create")
	}
	return o, nil
}

// GetByID fetches an organization by ID.
func (r *Repository) GetByID(ctx context.Context, id string) (Organization, error) {
	o, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return Organization{}, postgres.Translate(err, "org: get")
	}
	return o, nil
}

// GetBySlug fetches by slug.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (Organization, error) {
	list, err := r.repo.Find(ctx, repository.Eq("slug", slug), "")
	if err != nil {
		return Organization{}, postgres.Translate(err, "org: get by slug")
	}
	if len(list) == 0 {
		return Organization{}, errors.NotFound("organization not found")
	}
	return list[0], nil
}

// Update updates name, avatar, settings, metadata, limits.
func (r *Repository) Update(ctx context.Context, id string, vals map[string]any) (Organization, error) {
	vals["updated_at"] = time.Now().UTC()
	o, err := r.repo.Update(ctx, id, vals)
	if err != nil {
		return Organization{}, postgres.Translate(err, "org: update")
	}
	return o, nil
}

// SoftDelete soft-deletes an organization.
func (r *Repository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.repo.SoftDelete(ctx, id)
	return postgres.Translate(err, "org: delete")
}

// ListForUser returns organizations the user is an active member of.
func (r *Repository) ListForUser(ctx context.Context, userID string) ([]Organization, error) {
	const q = `
SELECT o.id, o.name, o.slug, o.avatar_url, o.plan_tier, o.billing_owner_id,
	o.settings, o.metadata, o.limits, o.created_at, o.updated_at, o.deleted_at
FROM identity_organizations o
JOIN identity_members m ON m.org_id = o.id
WHERE m.user_id = $1 AND m.status = 'active' AND o.deleted_at IS NULL
ORDER BY o.name`
	rows, err := r.db.Conn(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, postgres.Translate(err, "org: list for user")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Organization])
}
