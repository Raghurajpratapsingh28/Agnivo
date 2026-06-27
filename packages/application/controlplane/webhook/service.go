package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	"github.com/agnivo/agnivo/packages/application/controlplane/gitrepo"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/hashx"
	"github.com/agnivo/agnivo/packages/platform/httpx"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
)

// Service processes inbound git webhooks.
type Service struct {
	db          *postgres.DB
	deployments *deployment.Service
	git         *gitrepo.Service
	events      *cpevents.Publisher
	secrets     config.WebhookSecrets
}

// NewService constructs a webhook service.
func NewService(db *postgres.DB, deployments *deployment.Service, git *gitrepo.Service, events *cpevents.Publisher, cfg *config.Config) *Service {
	return &Service{db: db, deployments: deployments, git: git, events: events, secrets: cfg.ControlPlane.Webhooks}
}

// GitHubPayload is a minimal GitHub push event payload.
type GitHubPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"head_commit"`
}

// Handle processes a provider webhook request.
func (s *Service) Handle(ctx context.Context, provider string, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return errors.InvalidArgument("failed to read webhook body")
	}
	deliveryID := httpx.Header(r, "X-GitHub-Delivery")
	if deliveryID == "" {
		deliveryID = httpx.Header(r, "X-Gitlab-Event-UUID")
	}
	if deliveryID == "" {
		deliveryID = httpx.Header(r, "X-Request-UUID")
	}
	if deliveryID == "" {
		deliveryID = idx.NewUUID()
	}
	if err := s.verify(provider, r, body); err != nil {
		return err
	}
	hash := sha256Hex(body)
	if err := s.recordDelivery(ctx, provider, deliveryID, httpx.Header(r, "X-GitHub-Event"), hash); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil
		}
		return err
	}
	switch strings.ToLower(provider) {
	case "github":
		return s.handleGitHub(ctx, r, body)
	case "gitlab":
		return s.handleGitLab(ctx, r, body)
	case "bitbucket":
		return s.handleBitbucket(ctx, r, body)
	default:
		return errors.InvalidArgument("unsupported webhook provider")
	}
}

func (s *Service) handleGitHub(ctx context.Context, r *http.Request, body []byte) error {
	event := httpx.Header(r, "X-GitHub-Event")
	if event != "push" && event != "release" && event != "create" {
		return nil
	}
	var p GitHubPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return errors.InvalidArgument("invalid github payload")
	}
	if p.Deleted {
		return nil
	}
	branch := strings.TrimPrefix(p.Ref, "refs/heads/")
	sha := p.After
	if sha == "" {
		sha = p.HeadCommit.ID
	}
	return s.triggerDeploy(ctx, p.Repository.CloneURL, deployment.DeployInput{
		CommitSHA: sha, CommitMessage: p.HeadCommit.Message,
		Branch: branch, Author: p.HeadCommit.Author.Name,
	})
}

func (s *Service) handleGitLab(ctx context.Context, r *http.Request, body []byte) error {
	var p struct {
		ObjectKind string `json:"object_kind"`
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Project    struct {
			GitHTTPURL string `json:"git_http_url"`
		} `json:"project"`
		Commits []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return errors.InvalidArgument("invalid gitlab payload")
	}
	if p.ObjectKind != "push" {
		return nil
	}
	branch := strings.TrimPrefix(p.Ref, "refs/heads/")
	sha, msg, author := p.After, "", ""
	if len(p.Commits) > 0 {
		last := p.Commits[len(p.Commits)-1]
		if sha == "" {
			sha = last.ID
		}
		msg, author = last.Message, last.Author.Name
	}
	return s.triggerDeploy(ctx, p.Project.GitHTTPURL, deployment.DeployInput{
		CommitSHA: sha, CommitMessage: msg, Branch: branch, Author: author,
	})
}

func (s *Service) handleBitbucket(ctx context.Context, r *http.Request, body []byte) error {
	var p struct {
		Push struct {
			Changes []struct {
				New struct {
					Name string `json:"name"`
					Target struct {
						Hash    string `json:"hash"`
						Message string `json:"message"`
						Author  struct {
							Raw string `json:"raw"`
						} `json:"author"`
					} `json:"target"`
				} `json:"new"`
			} `json:"changes"`
		} `json:"push"`
		Repository struct {
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return errors.InvalidArgument("invalid bitbucket payload")
	}
	if len(p.Push.Changes) == 0 {
		return nil
	}
	ch := p.Push.Changes[0].New
	return s.triggerDeploy(ctx, p.Repository.Links.HTML.Href, deployment.DeployInput{
		CommitSHA: ch.Target.Hash, CommitMessage: ch.Target.Message,
		Branch: ch.Name, Author: ch.Target.Author.Raw,
	})
}

func (s *Service) triggerDeploy(ctx context.Context, repoURL string, in deployment.DeployInput) error {
	// Webhook secret header lookup: X-Hub-Signature-256 verified separately; repo matched by URL prefix.
	repo, err := s.findRepoByURL(ctx, repoURL)
	if err != nil {
		return nil // silently ignore unknown repos to prevent enumeration
	}
	d, err := s.deployments.DeployFromWebhook(ctx, repo.OrgID, repo.ProjectID, in)
	if err != nil {
		return err
	}
	_ = s.events.PublishAsync(ctx, cpevents.WebhookReceived, cpevents.Meta{
		OrgID: repo.OrgID, ProjectID: repo.ProjectID, AggregateID: d.ID,
		CorrelationID: logger.CorrelationID(ctx),
	}, map[string]string{"deployment_id": d.ID})
	return nil
}

func (s *Service) findRepoByURL(ctx context.Context, url string) (gitrepo.Repository, error) {
	const q = `SELECT id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
		metadata, connected_at, updated_at, disconnected_at
		FROM controlplane_git_repositories
		WHERE disconnected_at IS NULL AND (repo_url=$1 OR clone_url=$1 OR repo_url LIKE $2) LIMIT 1`
	row := s.db.Conn(ctx).QueryRow(ctx, q, url, url+"%")
	var r gitrepo.Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.OrgID, &r.Provider, &r.RepoURL, &r.CloneURL,
		&r.DefaultBranch, &r.IsPrivate, &r.Metadata, &r.ConnectedAt, &r.UpdatedAt, &r.DisconnectedAt)
	if err != nil {
		return gitrepo.Repository{}, postgres.Translate(err, "webhook: find repo")
	}
	return r, nil
}

func (s *Service) verify(provider string, r *http.Request, body []byte) error {
	switch strings.ToLower(provider) {
	case "github":
		sig := httpx.Header(r, "X-Hub-Signature-256")
		if sig == "" || s.secrets.GitHub == "" {
			return nil
		}
		expected := "sha256=" + hmacSHA256(s.secrets.GitHub, body)
		if !hashx.ConstantTimeEqualString(sig, expected) {
			return errors.Unauthenticated("invalid github signature")
		}
	case "gitlab":
		token := httpx.Header(r, "X-Gitlab-Token")
		if s.secrets.GitLab != "" && token != s.secrets.GitLab {
			return errors.Unauthenticated("invalid gitlab token")
		}
	case "bitbucket":
		// Bitbucket uses optional shared secret in query or header depending on setup.
		if s.secrets.Bitbucket != "" {
			if httpx.Header(r, "X-Hook-UUID") == "" && httpx.QueryString(r, "secret") != s.secrets.Bitbucket {
				return errors.Unauthenticated("invalid bitbucket secret")
			}
		}
	}
	return nil
}

var errDuplicate = errors.AlreadyExists("duplicate webhook delivery")

func (s *Service) recordDelivery(ctx context.Context, provider, deliveryID, eventType, payloadHash string) error {
	const q = `INSERT INTO controlplane_webhook_deliveries (id, provider, delivery_id, event_type, payload_hash, received_at)
		VALUES ($1,$2,$3,$4,$5,now())`
	_, err := s.db.Conn(ctx).Exec(ctx, q, idx.NewUUID(), provider, deliveryID, eventType, payloadHash)
	if err != nil {
		tr := postgres.Translate(err, "webhook: record delivery")
		if postgres.IsUniqueViolation(tr) {
			return errDuplicate
		}
		return tr
	}
	return nil
}

func hmacSHA256(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
