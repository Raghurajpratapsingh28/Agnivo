package project

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/repository"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/strx"
	"github.com/jackc/pgx/v5"
)

// Repository persists projects.
type Repository struct {
	repo *repository.Repository[Project]
	db   *postgres.DB
}

// NewRepository constructs a project repository.
func NewRepository(db *postgres.DB) *Repository {
	return &Repository{
		db: db,
		repo: repository.New[Project](db, "controlplane_projects",
			repository.WithSoftDelete("deleted_at"),
			repository.WithColumns(
				"id", "org_id", "name", "slug", "description", "repo_url", "repo_provider",
				"branch", "default_runtime", "framework", "build_method", "region",
				"sleep_config", "visibility", "status", "labels", "tags", "metadata",
				"created_by", "created_at", "updated_at", "archived_at", "deleted_at",
			),
		),
	}
}

// NormalizeSlug slugifies a project slug.
func NormalizeSlug(s string) string { return strx.Slugify(s) }

// Create inserts a new project.
func (r *Repository) Create(ctx context.Context, p Project) (Project, error) {
	sleep, _ := json.Marshal(map[string]any{})
	labels, _ := json.Marshal([]string{})
	meta, _ := json.Marshal(map[string]any{})
	now := time.Now().UTC()
	if len(p.SleepConfig) == 0 {
		p.SleepConfig = sleep
	}
	if len(p.Labels) == 0 {
		p.Labels = labels
	}
	if len(p.Metadata) == 0 {
		p.Metadata = meta
	}
	if p.ID == "" {
		p.ID = idx.NewUUID()
	}
	if p.Status == "" {
		p.Status = StatusActive
	}
	if p.Visibility == "" {
		p.Visibility = VisibilityPrivate
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	if len(p.Tags) == 0 {
		p.Tags = []string{}
	}
	out, err := r.repo.Insert(ctx, map[string]any{
		"id": p.ID, "org_id": p.OrgID, "name": p.Name, "slug": p.Slug,
		"description": p.Description, "repo_url": p.RepoURL, "repo_provider": p.RepoProvider,
		"branch": p.Branch, "default_runtime": p.DefaultRuntime, "framework": p.Framework,
		"build_method": p.BuildMethod, "region": p.Region, "sleep_config": p.SleepConfig,
		"visibility": p.Visibility, "status": p.Status, "labels": p.Labels,
		"tags": p.Tags, "metadata": p.Metadata, "created_by": p.CreatedBy,
		"created_at": now, "updated_at": now,
	})
	if err != nil {
		return Project{}, postgres.Translate(err, "project: create")
	}
	return out, nil
}

// GetByID fetches a live project scoped to org.
func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Project, error) {
	list, err := r.repo.Find(ctx, repository.And(
		repository.Eq("id", id),
		repository.Eq("org_id", orgID),
	), "")
	if err != nil {
		return Project{}, postgres.Translate(err, "project: get")
	}
	if len(list) == 0 {
		return Project{}, errors.NotFound("project not found")
	}
	return list[0], nil
}

// GetBySlug fetches by org and slug.
func (r *Repository) GetBySlug(ctx context.Context, orgID, slug string) (Project, error) {
	list, err := r.repo.Find(ctx, repository.And(
		repository.Eq("org_id", orgID),
		repository.Eq("slug", slug),
	), "")
	if err != nil {
		return Project{}, postgres.Translate(err, "project: get by slug")
	}
	if len(list) == 0 {
		return Project{}, errors.NotFound("project not found")
	}
	return list[0], nil
}

// ListByOrg returns live projects for an organization.
func (r *Repository) ListByOrg(ctx context.Context, orgID string, includeArchived bool) ([]Project, error) {
	q := `SELECT id, org_id, name, slug, description, repo_url, repo_provider, branch,
		default_runtime, framework, build_method, region, sleep_config, visibility, status,
		labels, tags, metadata, created_by, created_at, updated_at, archived_at, deleted_at
		FROM controlplane_projects WHERE org_id=$1 AND deleted_at IS NULL`
	if !includeArchived {
		q += ` AND status='active'`
	}
	q += ` ORDER BY name`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, postgres.Translate(err, "project: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Project])
}

// Update applies partial updates.
func (r *Repository) Update(ctx context.Context, orgID, id string, vals map[string]any) (Project, error) {
	if _, err := r.GetByID(ctx, orgID, id); err != nil {
		return Project{}, err
	}
	vals["updated_at"] = time.Now().UTC()
	out, err := r.repo.Update(ctx, id, vals)
	if err != nil {
		return Project{}, postgres.Translate(err, "project: update")
	}
	return out, nil
}

// Archive marks a project archived.
func (r *Repository) Archive(ctx context.Context, orgID, id string) (Project, error) {
	now := time.Now().UTC()
	return r.Update(ctx, orgID, id, map[string]any{
		"status": StatusArchived, "archived_at": now,
	})
}

// Restore unarchives a project.
func (r *Repository) Restore(ctx context.Context, orgID, id string) (Project, error) {
	return r.Update(ctx, orgID, id, map[string]any{
		"status": StatusActive, "archived_at": nil,
	})
}

// SoftDelete soft-deletes a project.
func (r *Repository) SoftDelete(ctx context.Context, orgID, id string) error {
	if _, err := r.GetByID(ctx, orgID, id); err != nil {
		return err
	}
	_, err := r.repo.SoftDelete(ctx, id)
	if err != nil {
		return postgres.Translate(err, "project: delete")
	}
	_, err = r.repo.Update(ctx, id, map[string]any{
		"status": StatusDeleted, "updated_at": time.Now().UTC(),
	})
	return postgres.Translate(err, "project: delete")
}

// Transfer moves a project to another organization (updates org_id).
func (r *Repository) Transfer(ctx context.Context, id, fromOrg, toOrg string) (Project, error) {
	if _, err := r.GetByID(ctx, fromOrg, id); err != nil {
		return Project{}, err
	}
	return r.Update(ctx, fromOrg, id, map[string]any{"org_id": toOrg})
}

// SlugExists reports whether slug is taken in org.
func (r *Repository) SlugExists(ctx context.Context, orgID, slug string, exceptID string) (bool, error) {
	conds := []repository.Condition{
		repository.Eq("org_id", orgID),
		repository.Eq("slug", slug),
	}
	if exceptID != "" {
		conds = append(conds, repository.Neq("id", exceptID))
	}
	n, err := r.repo.Count(ctx, repository.And(conds...))
	return n > 0, postgres.Translate(err, "project: slug exists")
}
