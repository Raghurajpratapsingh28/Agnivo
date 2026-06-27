package gitrepo

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

// Provider is a git hosting provider.
type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
	ProviderGeneric   Provider = "generic"
)

// Repository is a connected git repository.
type Repository struct {
	ID             string          `db:"id" json:"id"`
	ProjectID      string          `db:"project_id" json:"project_id"`
	OrgID          string          `db:"org_id" json:"org_id"`
	Provider       Provider        `db:"provider" json:"provider"`
	RepoURL        string          `db:"repo_url" json:"repo_url"`
	CloneURL       string          `db:"clone_url" json:"clone_url"`
	DefaultBranch  string          `db:"default_branch" json:"default_branch"`
	IsPrivate      bool            `db:"is_private" json:"is_private"`
	AccessTokenEnc []byte          `db:"-" json:"-"`
	DeployKeyEnc   []byte          `db:"-" json:"-"`
	WebhookSecret  string          `db:"webhook_secret" json:"-"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	ConnectedAt    time.Time       `db:"connected_at" json:"connected_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
	DisconnectedAt *time.Time      `db:"disconnected_at" json:"disconnected_at,omitempty"`
}

// Store persists git repositories.
type Store struct {
	db *postgres.DB
}

// NewStore constructs a git repository store.
func NewStore(db *postgres.DB) *Store { return &Store{db: db} }

// ConnectInput connects a repository to a project.
type ConnectInput struct {
	Provider      Provider `json:"provider" validate:"required,oneof=github gitlab bitbucket generic"`
	RepoURL       string   `json:"repo_url" validate:"required,git_repo"`
	CloneURL      string   `json:"clone_url" validate:"omitempty,git_repo"`
	DefaultBranch string   `json:"default_branch" validate:"omitempty,max=255"`
	IsPrivate     bool     `json:"is_private"`
	AccessToken   string   `json:"access_token" validate:"omitempty,min=1"`
	DeployKey     string   `json:"deploy_key" validate:"omitempty,min=1"`
}

// Service handles git repository operations.
type Service struct {
	store  *Store
	vault  *crypto.Vault
	events *cpevents.Publisher
	audit  *audit.Logger
}

// NewService constructs a git repository service.
func NewService(store *Store, vault *crypto.Vault, events *cpevents.Publisher, auditLog *audit.Logger) *Service {
	return &Service{store: store, vault: vault, events: events, audit: auditLog}
}

// Connect connects a git repository to a project.
func (s *Service) Connect(ctx context.Context, orgID, projectID, userID string, in ConnectInput, ip, ua string) (Repository, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Repository{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite); err != nil {
		return Repository{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	var tokenEnc, keyEnc []byte
	var err error
	if in.AccessToken != "" {
		tokenEnc, err = s.vault.Encrypt([]byte(in.AccessToken), aad)
		if err != nil {
			return Repository{}, err
		}
	}
	if in.DeployKey != "" {
		keyEnc, err = s.vault.Encrypt([]byte(in.DeployKey), aad)
		if err != nil {
			return Repository{}, err
		}
	}
	branch := in.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	webhookSecret := idx.Token(32)
	repo, err := s.store.Connect(ctx, Repository{
		ID: idx.NewUUID(), ProjectID: projectID, OrgID: orgID, Provider: in.Provider,
		RepoURL: in.RepoURL, CloneURL: in.CloneURL, DefaultBranch: branch,
		IsPrivate: in.IsPrivate, AccessTokenEnc: tokenEnc, DeployKeyEnc: keyEnc,
		WebhookSecret: webhookSecret,
	})
	if err != nil {
		return Repository{}, err
	}
	repo.WebhookSecret = ""
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "repository.connect",
		ResourceType: "repository", ResourceID: repo.ID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.RepositoryConnected, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: repo.ID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, repo)
	return repo, nil
}

// Get returns the connected repository for a project.
func (s *Service) Get(ctx context.Context, orgID, projectID string) (Repository, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Repository{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectRead); err != nil {
		return Repository{}, err
	}
	return s.store.GetByProject(ctx, orgID, projectID)
}

// Disconnect removes the repository connection.
func (s *Service) Disconnect(ctx context.Context, orgID, projectID, userID, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite); err != nil {
		return err
	}
	repo, err := s.store.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return err
	}
	if err := s.store.Disconnect(ctx, repo.ID); err != nil {
		return err
	}
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "repository.disconnect",
		ResourceType: "repository", ResourceID: repo.ID, IPAddress: ip, UserAgent: ua})
	_ = s.events.PublishAsync(ctx, cpevents.RepositoryDisconnected, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: repo.ID, ActorID: userID,
	}, nil)
	return nil
}

// UpdateInput updates repository settings.
type UpdateInput struct {
	DefaultBranch *string `json:"default_branch" validate:"omitempty,max=255"`
	CloneURL      *string `json:"clone_url" validate:"omitempty,git_repo"`
	IsPrivate     *bool   `json:"is_private"`
	AccessToken   *string `json:"access_token" validate:"omitempty,min=1"`
}

// Update updates repository configuration.
func (s *Service) Update(ctx context.Context, orgID, projectID, userID string, in UpdateInput, ip, ua string) (Repository, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Repository{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite); err != nil {
		return Repository{}, err
	}
	repo, err := s.store.GetByProject(ctx, orgID, projectID)
	if err != nil {
		return Repository{}, err
	}
	aad := crypto.AAD(orgID, projectID)
	vals := map[string]any{"updated_at": time.Now().UTC()}
	if in.DefaultBranch != nil {
		vals["default_branch"] = *in.DefaultBranch
	}
	if in.CloneURL != nil {
		vals["clone_url"] = *in.CloneURL
	}
	if in.IsPrivate != nil {
		vals["is_private"] = *in.IsPrivate
	}
	if in.AccessToken != nil && *in.AccessToken != "" {
		enc, err := s.vault.Encrypt([]byte(*in.AccessToken), aad)
		if err != nil {
			return Repository{}, err
		}
		vals["access_token_enc"] = enc
	}
	out, err := s.store.Update(ctx, repo.ID, vals)
	if err != nil {
		return Repository{}, err
	}
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "repository.update",
		ResourceType: "repository", ResourceID: repo.ID, IPAddress: ip, UserAgent: ua})
	return out, nil
}

// FindByWebhookSecret locates a repo by its webhook secret (for webhook ingestion).
func (s *Service) FindByWebhookSecret(ctx context.Context, secret string) (Repository, error) {
	return s.store.GetByWebhookSecret(ctx, secret)
}

// Connect inserts or replaces a repository connection.
func (st *Store) Connect(ctx context.Context, r Repository) (Repository, error) {
	meta, _ := json.Marshal(map[string]any{})
	now := time.Now().UTC()
	err := st.db.Transact(ctx, func(ctx context.Context) error {
		_, err := st.db.Conn(ctx).Exec(ctx,
			`UPDATE controlplane_git_repositories SET disconnected_at=now() WHERE project_id=$1 AND disconnected_at IS NULL`, r.ProjectID)
		if err != nil {
			return postgres.Translate(err, "gitrepo: disconnect existing")
		}
		const q = `INSERT INTO controlplane_git_repositories
			(id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
			access_token_enc, deploy_key_enc, webhook_secret, metadata, connected_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
			RETURNING id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
			metadata, connected_at, updated_at, disconnected_at`
		row := st.db.Conn(ctx).QueryRow(ctx, q, r.ID, r.ProjectID, r.OrgID, r.Provider, r.RepoURL,
			r.CloneURL, r.DefaultBranch, r.IsPrivate, r.AccessTokenEnc, r.DeployKeyEnc,
			r.WebhookSecret, meta, now)
		_, err = scanRepo(row)
		return err
	})
	if err != nil {
		return Repository{}, err
	}
	return st.GetByProject(ctx, r.OrgID, r.ProjectID)
}

func (st *Store) GetByProject(ctx context.Context, orgID, projectID string) (Repository, error) {
	const q = `SELECT id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
		metadata, connected_at, updated_at, disconnected_at
		FROM controlplane_git_repositories
		WHERE project_id=$1 AND org_id=$2 AND disconnected_at IS NULL LIMIT 1`
	row := st.db.Conn(ctx).QueryRow(ctx, q, projectID, orgID)
	return scanRepoPublic(row)
}

func (st *Store) GetByWebhookSecret(ctx context.Context, secret string) (Repository, error) {
	const q = `SELECT id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
		webhook_secret, metadata, connected_at, updated_at, disconnected_at
		FROM controlplane_git_repositories WHERE webhook_secret=$1 AND disconnected_at IS NULL LIMIT 1`
	row := st.db.Conn(ctx).QueryRow(ctx, q, secret)
	var r Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.OrgID, &r.Provider, &r.RepoURL, &r.CloneURL,
		&r.DefaultBranch, &r.IsPrivate, &r.WebhookSecret, &r.Metadata, &r.ConnectedAt, &r.UpdatedAt, &r.DisconnectedAt)
	if err != nil {
		return Repository{}, postgres.Translate(err, "gitrepo: get by secret")
	}
	return r, nil
}

func (st *Store) Disconnect(ctx context.Context, id string) error {
	tag, err := st.db.Conn(ctx).Exec(ctx,
		`UPDATE controlplane_git_repositories SET disconnected_at=now(), updated_at=now() WHERE id=$1 AND disconnected_at IS NULL`, id)
	if err != nil {
		return postgres.Translate(err, "gitrepo: disconnect")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("repository not found")
	}
	return nil
}

func (st *Store) Update(ctx context.Context, id string, vals map[string]any) (Repository, error) {
	// Simple dynamic update for a small set of fields.
	if branch, ok := vals["default_branch"]; ok {
		_, _ = st.db.Conn(ctx).Exec(ctx, `UPDATE controlplane_git_repositories SET default_branch=$2, updated_at=now() WHERE id=$1`, id, branch)
	}
	if url, ok := vals["clone_url"]; ok {
		_, _ = st.db.Conn(ctx).Exec(ctx, `UPDATE controlplane_git_repositories SET clone_url=$2, updated_at=now() WHERE id=$1`, id, url)
	}
	if priv, ok := vals["is_private"]; ok {
		_, _ = st.db.Conn(ctx).Exec(ctx, `UPDATE controlplane_git_repositories SET is_private=$2, updated_at=now() WHERE id=$1`, id, priv)
	}
	if tok, ok := vals["access_token_enc"]; ok {
		_, _ = st.db.Conn(ctx).Exec(ctx, `UPDATE controlplane_git_repositories SET access_token_enc=$2, updated_at=now() WHERE id=$1`, id, tok)
	}
	const q = `SELECT id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
		metadata, connected_at, updated_at, disconnected_at FROM controlplane_git_repositories WHERE id=$1`
	row := st.db.Conn(ctx).QueryRow(ctx, q, id)
	return scanRepoPublic(row)
}

func scanRepo(row pgx.Row) (Repository, error) {
	var r Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.OrgID, &r.Provider, &r.RepoURL, &r.CloneURL,
		&r.DefaultBranch, &r.IsPrivate, &r.Metadata, &r.ConnectedAt, &r.UpdatedAt, &r.DisconnectedAt)
	return r, err
}

func scanRepoPublic(row pgx.Row) (Repository, error) {
	var r Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.OrgID, &r.Provider, &r.RepoURL, &r.CloneURL,
		&r.DefaultBranch, &r.IsPrivate, &r.Metadata, &r.ConnectedAt, &r.UpdatedAt, &r.DisconnectedAt)
	if err != nil {
		return Repository{}, postgres.Translate(err, "gitrepo: get")
	}
	return r, nil
}
