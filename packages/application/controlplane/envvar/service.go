package envvar

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/controlplane/crypto"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"github.com/jackc/pgx/v5"
)

// Scope is the environment scope.
type Scope string

const (
	ScopeDevelopment Scope = "development"
	ScopePreview     Scope = "preview"
	ScopeProduction  Scope = "production"
)

// Variable is an environment variable.
type Variable struct {
	ID          string    `db:"id" json:"id"`
	OrgID       string    `db:"org_id" json:"org_id"`
	ProjectID   string    `db:"project_id" json:"project_id"`
	Key         string    `db:"key" json:"key"`
	ValueEnc    []byte    `db:"-" json:"-"`
	Environment Scope     `db:"environment" json:"environment"`
	IsSecret    bool      `db:"is_secret" json:"is_secret"`
	Version     int       `db:"version" json:"version"`
	CreatedBy   string    `db:"created_by" json:"created_by"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// PublicView is the API representation (masked when secret).
type PublicView struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Environment Scope  `json:"environment"`
	IsSecret    bool   `json:"is_secret"`
	Version     int    `json:"version"`
}

// Repository persists environment variables.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an env var repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Service handles environment variable business logic.
type Service struct {
	repo   *Repository
	vault  *crypto.Vault
	events *cpevents.Publisher
	audit  *audit.Logger
}

// NewService constructs an env var service.
func NewService(repo *Repository, vault *crypto.Vault, events *cpevents.Publisher, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, vault: vault, events: events, audit: auditLog}
}

// CreateInput creates an environment variable.
type CreateInput struct {
	Key         string `json:"key" validate:"required,env_var"`
	Value       string `json:"value" validate:"required"`
	Environment Scope  `json:"environment" validate:"required,oneof=development preview production"`
	IsSecret    bool   `json:"is_secret"`
}

// Create stores an encrypted environment variable.
func (s *Service) Create(ctx context.Context, orgID, projectID, userID string, in CreateInput, ip, ua string) (PublicView, error) {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return PublicView{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	enc, err := s.vault.Encrypt([]byte(in.Value), aad)
	if err != nil {
		return PublicView{}, err
	}
	v, err := s.repo.Create(ctx, Variable{
		ID: idx.NewUUID(), OrgID: orgID, ProjectID: projectID, Key: in.Key,
		ValueEnc: enc, Environment: in.Environment, IsSecret: in.IsSecret, CreatedBy: userID,
	})
	if err != nil {
		return PublicView{}, err
	}
	s.record(ctx, userID, orgID, projectID, v.ID, ip, ua)
	return s.toView(v, in.Value, false), nil
}

// UpdateInput updates an environment variable.
type UpdateInput struct {
	Value string `json:"value" validate:"required"`
}

// Update updates value and creates a version record.
func (s *Service) Update(ctx context.Context, orgID, projectID, varID, userID string, in UpdateInput, ip, ua string) (PublicView, error) {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return PublicView{}, err
	}
	v, err := s.repo.Get(ctx, orgID, projectID, varID)
	if err != nil {
		return PublicView{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	enc, err := s.vault.Encrypt([]byte(in.Value), aad)
	if err != nil {
		return PublicView{}, err
	}
	v, err = s.repo.UpdateValue(ctx, v.ID, enc, userID)
	if err != nil {
		return PublicView{}, err
	}
	s.record(ctx, userID, orgID, projectID, v.ID, ip, ua)
	return s.toView(v, in.Value, false), nil
}

// Delete soft-deletes an environment variable.
func (s *Service) Delete(ctx context.Context, orgID, projectID, varID, userID, ip, ua string) error {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, orgID, projectID, varID); err != nil {
		return err
	}
	s.record(ctx, userID, orgID, projectID, varID, ip, ua)
	return nil
}

// List returns environment variables (values masked for secrets).
func (s *Service) List(ctx context.Context, orgID, projectID string, env Scope) ([]PublicView, error) {
	if err := s.requireRead(ctx, orgID); err != nil {
		return nil, err
	}
	vars, err := s.repo.List(ctx, orgID, projectID, env)
	if err != nil {
		return nil, err
	}
	out := make([]PublicView, len(vars))
	for i, v := range vars {
		val := ""
		if !v.IsSecret {
			plain, err := s.vault.Decrypt(v.ValueEnc, crypto.AAD(orgID, projectID))
			if err == nil {
				val = string(plain)
			}
		} else {
			val = "********"
		}
		out[i] = s.toView(v, val, v.IsSecret)
	}
	return out, nil
}

// Import bulk-imports variables.
func (s *Service) Import(ctx context.Context, orgID, projectID, userID string, items []CreateInput, ip, ua string) ([]PublicView, error) {
	out := make([]PublicView, 0, len(items))
	for _, item := range items {
		v, err := s.Create(ctx, orgID, projectID, userID, item, ip, ua)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) toView(v Variable, value string, masked bool) PublicView {
	if masked {
		value = "********"
	}
	return PublicView{ID: v.ID, Key: v.Key, Value: value, Environment: v.Environment, IsSecret: v.IsSecret, Version: v.Version}
}

func (s *Service) requireRead(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectRead)
}

func (s *Service) requireWrite(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite)
}

func (s *Service) record(ctx context.Context, userID, orgID, projectID, varID, ip, ua string) {
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "env.update",
		ResourceType: "env_var", ResourceID: varID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.EnvironmentUpdated, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: varID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, map[string]string{"id": varID})
}

func (r *Repository) Create(ctx context.Context, v Variable) (Variable, error) {
	now := time.Now().UTC()
	const q = `INSERT INTO controlplane_env_vars
		(id, org_id, project_id, key, value_enc, environment, is_secret, version, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,1,$8,$9,$9)`
	_, err := r.db.Conn(ctx).Exec(ctx, q, v.ID, v.OrgID, v.ProjectID, v.Key, v.ValueEnc,
		v.Environment, v.IsSecret, v.CreatedBy, now)
	if err != nil {
		return Variable{}, postgres.Translate(err, "envvar: create")
	}
	return r.Get(ctx, v.OrgID, v.ProjectID, v.ID)
}

func (r *Repository) Get(ctx context.Context, orgID, projectID, id string) (Variable, error) {
	const q = `SELECT id, org_id, project_id, key, value_enc, environment, is_secret, version, created_by, created_at, updated_at
		FROM controlplane_env_vars WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`
	row := r.db.Conn(ctx).QueryRow(ctx, q, id, orgID, projectID)
	return scanVar(row)
}

func (r *Repository) List(ctx context.Context, orgID, projectID string, env Scope) ([]Variable, error) {
	q := `SELECT id, org_id, project_id, key, value_enc, environment, is_secret, version, created_by, created_at, updated_at
		FROM controlplane_env_vars WHERE org_id=$1 AND project_id=$2 AND deleted_at IS NULL`
	args := []any{orgID, projectID}
	if env != "" {
		q += ` AND environment=$3`
		args = append(args, env)
	}
	q += ` ORDER BY key`
	rows, err := r.db.Conn(ctx).Query(ctx, q, args...)
	if err != nil {
		return nil, postgres.Translate(err, "envvar: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Variable])
}

func (r *Repository) UpdateValue(ctx context.Context, id string, enc []byte, userID string) (Variable, error) {
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		row := r.db.Conn(ctx).QueryRow(ctx,
			`SELECT id, org_id, project_id, key, value_enc, environment, is_secret, version, created_by, created_at, updated_at
			FROM controlplane_env_vars WHERE id=$1 AND deleted_at IS NULL`, id)
		v, err := scanVar(row)
		if err != nil {
			return err
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`INSERT INTO controlplane_env_var_versions (id, env_var_id, version, value_enc, changed_by, created_at)
			VALUES ($1,$2,$3,$4,$5,now())`, idx.NewUUID(), id, v.Version, v.ValueEnc, userID)
		if err != nil {
			return postgres.Translate(err, "envvar: version")
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`UPDATE controlplane_env_vars SET value_enc=$2, version=$3, updated_at=now() WHERE id=$1`, id, enc, v.Version+1)
		return postgres.Translate(err, "envvar: update")
	})
	if err != nil {
		return Variable{}, err
	}
	row := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT id, org_id, project_id, key, value_enc, environment, is_secret, version, created_by, created_at, updated_at
		FROM controlplane_env_vars WHERE id=$1`, id)
	return scanVar(row)
}

func (r *Repository) Delete(ctx context.Context, orgID, projectID, id string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE controlplane_env_vars SET deleted_at=now(), updated_at=now()
		WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`, id, orgID, projectID)
	if err != nil {
		return postgres.Translate(err, "envvar: delete")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("environment variable not found")
	}
	return nil
}

func scanVar(row pgx.Row) (Variable, error) {
	var v Variable
	err := row.Scan(&v.ID, &v.OrgID, &v.ProjectID, &v.Key, &v.ValueEnc, &v.Environment,
		&v.IsSecret, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return Variable{}, postgres.Translate(err, "envvar: scan")
	}
	return v, nil
}
