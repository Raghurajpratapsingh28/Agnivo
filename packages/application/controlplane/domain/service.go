package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpjobs"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"github.com/jackc/pgx/v5"
)

// Type classifies a domain.
type Type string

const (
	TypePlatform Type = "platform"
	TypeCustom   Type = "custom"
	TypeWildcard Type = "wildcard"
	TypePreview  Type = "preview"
)

// Domain is a project domain binding.
type Domain struct {
	ID                 string          `db:"id" json:"id"`
	OrgID              string          `db:"org_id" json:"org_id"`
	ProjectID          string          `db:"project_id" json:"project_id"`
	Hostname           string          `db:"hostname" json:"hostname"`
	DomainType         Type            `db:"domain_type" json:"domain_type"`
	IsPrimary          bool            `db:"is_primary" json:"is_primary"`
	IsPreview          bool            `db:"is_preview" json:"is_preview"`
	VerificationStatus string          `db:"verification_status" json:"verification_status"`
	SSLStatus          string          `db:"ssl_status" json:"ssl_status"`
	RedirectTo         string          `db:"redirect_to" json:"redirect_to,omitempty"`
	Metadata           json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedAt          time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at" json:"updated_at"`
}

// Repository persists domains.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a domain repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Service handles domain management.
type Service struct {
	repo   *Repository
	jobs   *cpjobs.Enqueuer
	events *cpevents.Publisher
	audit  *audit.Logger
}

// NewService constructs a domain service.
func NewService(repo *Repository, jobs *cpjobs.Enqueuer, events *cpevents.Publisher, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, jobs: jobs, events: events, audit: auditLog}
}

// CreateInput adds a domain.
type CreateInput struct {
	Hostname   string `json:"hostname" validate:"required,domain"`
	DomainType Type   `json:"domain_type" validate:"omitempty,oneof=platform custom wildcard preview"`
	IsPrimary  bool   `json:"is_primary"`
	IsPreview  bool   `json:"is_preview"`
	RedirectTo string `json:"redirect_to" validate:"omitempty,domain"`
}

// Create registers a domain and enqueues verification.
func (s *Service) Create(ctx context.Context, orgID, projectID, userID string, in CreateInput, ip, ua string) (Domain, error) {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return Domain{}, err
	}
	dtype := in.DomainType
	if dtype == "" {
		dtype = TypeCustom
	}
	meta, _ := json.Marshal(map[string]any{})
	d, err := s.repo.Create(ctx, Domain{
		ID: idx.NewUUID(), OrgID: orgID, ProjectID: projectID, Hostname: in.Hostname,
		DomainType: dtype, IsPrimary: in.IsPrimary, IsPreview: in.IsPreview,
		RedirectTo: in.RedirectTo, VerificationStatus: "pending", SSLStatus: "pending", Metadata: meta,
	})
	if err != nil {
		return Domain{}, err
	}
	payload := cpjobs.Payload{OrgID: orgID, ProjectID: projectID, DomainID: d.ID, UserID: userID}
	_, _ = s.jobs.EnqueueDomainVerify(ctx, payload)
	_, _ = s.jobs.EnqueueSSLRequest(ctx, payload)
	s.recordAdd(ctx, userID, orgID, projectID, d.ID, ip, ua)
	return d, nil
}

// List returns project domains.
func (s *Service) List(ctx context.Context, orgID, projectID string) ([]Domain, error) {
	if err := s.requireRead(ctx, orgID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, orgID, projectID)
}

// Delete removes a domain.
func (s *Service) Delete(ctx context.Context, orgID, projectID, domainID, userID, ip, ua string) error {
	if err := s.requireWrite(ctx, orgID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, orgID, projectID, domainID); err != nil {
		return err
	}
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "domain.remove",
		ResourceType: "domain", ResourceID: domainID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.DomainRemoved, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: domainID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, map[string]string{"id": domainID})
	return nil
}

func (s *Service) recordAdd(ctx context.Context, userID, orgID, projectID, domainID, ip, ua string) {
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "domain.add",
		ResourceType: "domain", ResourceID: domainID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.DomainAdded, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: domainID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, map[string]string{"id": domainID})
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

func (r *Repository) Create(ctx context.Context, d Domain) (Domain, error) {
	now := time.Now().UTC()
	if d.IsPrimary {
		_, _ = r.db.Conn(ctx).Exec(ctx,
			`UPDATE controlplane_domains SET is_primary=false, updated_at=now()
			WHERE project_id=$1 AND deleted_at IS NULL`, d.ProjectID)
	}
	const q = `INSERT INTO controlplane_domains
		(id, org_id, project_id, hostname, domain_type, is_primary, is_preview,
		verification_status, ssl_status, redirect_to, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		RETURNING id, org_id, project_id, hostname, domain_type, is_primary, is_preview,
		verification_status, ssl_status, redirect_to, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q, d.ID, d.OrgID, d.ProjectID, d.Hostname, d.DomainType,
		d.IsPrimary, d.IsPreview, d.VerificationStatus, d.SSLStatus, d.RedirectTo, d.Metadata, now)
	return scanDomain(row)
}

func (r *Repository) List(ctx context.Context, orgID, projectID string) ([]Domain, error) {
	const q = `SELECT id, org_id, project_id, hostname, domain_type, is_primary, is_preview,
		verification_status, ssl_status, redirect_to, metadata, created_at, updated_at
		FROM controlplane_domains WHERE org_id=$1 AND project_id=$2 AND deleted_at IS NULL ORDER BY hostname`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID)
	if err != nil {
		return nil, postgres.Translate(err, "domain: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Domain])
}

func (r *Repository) Delete(ctx context.Context, orgID, projectID, id string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE controlplane_domains SET deleted_at=now(), updated_at=now()
		WHERE id=$1 AND org_id=$2 AND project_id=$3 AND deleted_at IS NULL`, id, orgID, projectID)
	if err != nil {
		return postgres.Translate(err, "domain: delete")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("domain not found")
	}
	return nil
}

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	err := row.Scan(&d.ID, &d.OrgID, &d.ProjectID, &d.Hostname, &d.DomainType, &d.IsPrimary,
		&d.IsPreview, &d.VerificationStatus, &d.SSLStatus, &d.RedirectTo, &d.Metadata, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Domain{}, postgres.Translate(err, "domain: scan")
	}
	return d, nil
}
