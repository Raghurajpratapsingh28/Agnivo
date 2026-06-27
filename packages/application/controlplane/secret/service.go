package secret

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpevents"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/audit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"github.com/jackc/pgx/v5"
)

// Secret is an encrypted project secret.
type Secret struct {
	ID          string          `db:"id" json:"id"`
	OrgID       string          `db:"org_id" json:"org_id"`
	ProjectID   string          `db:"project_id" json:"project_id"`
	Name        string          `db:"name" json:"name"`
	ValueEnc    []byte          `db:"-" json:"-"`
	Environment string          `db:"environment" json:"environment"`
	Version     int             `db:"version" json:"version"`
	DisabledAt  *time.Time      `db:"disabled_at" json:"disabled_at,omitempty"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedBy   string          `db:"created_by" json:"created_by"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

// PublicView never exposes plaintext after creation.
type PublicView struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Environment string          `json:"environment"`
	Version     int             `json:"version"`
	Disabled    bool            `json:"disabled"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Repository persists secrets.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a secret repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Service handles secret management.
type Service struct {
	repo   *Repository
	vault  *crypto.Vault
	events *cpevents.Publisher
	audit  *audit.Logger
}

// NewService constructs a secret service.
func NewService(repo *Repository, vault *crypto.Vault, events *cpevents.Publisher, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, vault: vault, events: events, audit: auditLog}
}

// CreateInput creates a secret (plaintext shown once in response only at handler level).
type CreateInput struct {
	Name        string          `json:"name" validate:"required,min=1,max=100"`
	Value       string          `json:"value" validate:"required,secret"`
	Environment string          `json:"environment" validate:"required,oneof=development preview production"`
	Metadata    json.RawMessage `json:"metadata"`
}

// Create stores an encrypted secret.
func (s *Service) Create(ctx context.Context, orgID, projectID, userID string, in CreateInput, ip, ua string) (PublicView, error) {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return PublicView{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	enc, err := s.vault.Encrypt([]byte(in.Value), aad)
	if err != nil {
		return PublicView{}, err
	}
	sec, err := s.repo.Create(ctx, Secret{
		ID: idx.NewUUID(), OrgID: orgID, ProjectID: projectID, Name: in.Name,
		ValueEnc: enc, Environment: in.Environment, Metadata: in.Metadata, CreatedBy: userID,
	})
	if err != nil {
		return PublicView{}, err
	}
	s.record(ctx, userID, orgID, projectID, sec.ID, ip, ua)
	return toView(sec), nil
}

// Rotate replaces secret value with a new version.
func (s *Service) Rotate(ctx context.Context, orgID, projectID, secretID, userID, newValue, ip, ua string) (PublicView, error) {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return PublicView{}, err
	}
	sec, err := s.repo.Get(ctx, orgID, projectID, secretID)
	if err != nil {
		return PublicView{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	enc, err := s.vault.Encrypt([]byte(newValue), aad)
	if err != nil {
		return PublicView{}, err
	}
	sec, err = s.repo.Rotate(ctx, sec.ID, enc, userID)
	if err != nil {
		return PublicView{}, err
	}
	s.record(ctx, userID, orgID, projectID, sec.ID, ip, ua)
	return toView(sec), nil
}

// Disable disables a secret.
func (s *Service) Disable(ctx context.Context, orgID, projectID, secretID, userID, ip, ua string) error {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return err
	}
	if err := s.repo.Disable(ctx, orgID, projectID, secretID); err != nil {
		return err
	}
	s.record(ctx, userID, orgID, projectID, secretID, ip, ua)
	return nil
}

// Delete soft-deletes a secret.
func (s *Service) Delete(ctx context.Context, orgID, projectID, secretID, userID, ip, ua string) error {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, orgID, projectID, secretID)
}

// List returns secrets without plaintext.
func (s *Service) List(ctx context.Context, orgID, projectID string) ([]PublicView, error) {
	if err := s.requireRead(ctx, orgID); err != nil {
		return nil, err
	}
	secs, err := s.repo.List(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]PublicView, len(secs))
	for i, sec := range secs {
		out[i] = toView(sec)
	}
	return out, nil
}

func toView(sec Secret) PublicView {
	return PublicView{
		ID: sec.ID, Name: sec.Name, Environment: sec.Environment,
		Version: sec.Version, Disabled: sec.DisabledAt != nil,
		Metadata: sec.Metadata, CreatedAt: sec.CreatedAt,
	}
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

func (s *Service) record(ctx context.Context, userID, orgID, projectID, secretID, ip, ua string) {
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "secret.update",
		ResourceType: "secret", ResourceID: secretID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.SecretUpdated, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: secretID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, map[string]string{"id": secretID})
}

func (r *Repository) Create(ctx context.Context, s Secret) (Secret, error) {
	if s.Metadata == nil {
		s.Metadata, _ = json.Marshal(map[string]any{})
	}
	now := time.Now().UTC()
	const q = `INSERT INTO controlplane_secrets
		(id, org_id, project_id, name, value_enc, environment, version, metadata, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,1,$7,$8,$9,$9)`
	_, err := r.db.Conn(ctx).Exec(ctx, q, s.ID, s.OrgID, s.ProjectID, s.Name, s.ValueEnc,
		s.Environment, s.Metadata, s.CreatedBy, now)
	if err != nil {
		return Secret{}, postgres.Translate(err, "secret: create")
	}
	return r.Get(ctx, s.OrgID, s.ProjectID, s.ID)
}

func (r *Repository) Get(ctx context.Context, orgID, projectID, id string) (Secret, error) {
	const q = `SELECT id, org_id, project_id, name, value_enc, environment, version, disabled_at, metadata, created_by, created_at, updated_at
		FROM controlplane_secrets WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`
	row := r.db.Conn(ctx).QueryRow(ctx, q, id, orgID, projectID)
	return scanSecret(row)
}

func (r *Repository) List(ctx context.Context, orgID, projectID string) ([]Secret, error) {
	const q = `SELECT id, org_id, project_id, name, value_enc, environment, version, disabled_at, metadata, created_by, created_at, updated_at
		FROM controlplane_secrets WHERE org_id=$1 AND project_id=$2 AND deleted_at IS NULL ORDER BY name`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID)
	if err != nil {
		return nil, postgres.Translate(err, "secret: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Secret])
}

func (r *Repository) Rotate(ctx context.Context, id string, enc []byte, userID string) (Secret, error) {
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		row := r.db.Conn(ctx).QueryRow(ctx,
			`SELECT id, org_id, project_id, name, value_enc, environment, version, disabled_at, metadata, created_by, created_at, updated_at
			FROM controlplane_secrets WHERE id=$1 AND deleted_at IS NULL`, id)
		s, err := scanSecret(row)
		if err != nil {
			return err
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`INSERT INTO controlplane_secret_versions (id, secret_id, version, value_enc, rotated_by, created_at)
			VALUES ($1,$2,$3,$4,$5,now())`, idx.NewUUID(), id, s.Version, s.ValueEnc, userID)
		if err != nil {
			return postgres.Translate(err, "secret: version")
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`UPDATE controlplane_secrets SET value_enc=$2, version=$3, updated_at=now(), disabled_at=NULL WHERE id=$1`, id, enc, s.Version+1)
		return postgres.Translate(err, "secret: rotate")
	})
	if err != nil {
		return Secret{}, err
	}
	row := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT id, org_id, project_id, name, value_enc, environment, version, disabled_at, metadata, created_by, created_at, updated_at
		FROM controlplane_secrets WHERE id=$1`, id)
	return scanSecret(row)
}

func (r *Repository) Disable(ctx context.Context, orgID, projectID, id string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE controlplane_secrets SET disabled_at=now(), updated_at=now()
		WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`, id, orgID, projectID)
	if err != nil {
		return postgres.Translate(err, "secret: disable")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("secret not found")
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, orgID, projectID, id string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE controlplane_secrets SET deleted_at=now(), updated_at=now()
		WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`, id, orgID, projectID)
	if err != nil {
		return postgres.Translate(err, "secret: delete")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("secret not found")
	}
	return nil
}

func scanSecret(row pgx.Row) (Secret, error) {
	var s Secret
	err := row.Scan(&s.ID, &s.OrgID, &s.ProjectID, &s.Name, &s.ValueEnc, &s.Environment,
		&s.Version, &s.DisabledAt, &s.Metadata, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return Secret{}, postgres.Translate(err, "secret: scan")
	}
	return s, nil
}
