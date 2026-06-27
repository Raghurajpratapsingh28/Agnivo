// Package preview manages ephemeral preview deployments:
// automatic hostname generation, SSL provisioning, TTL-based cleanup,
// and isolation from production routes.
package preview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"go.uber.org/zap"
)

// CaddyPreview abstracts the Caddy operations needed for preview routing.
type CaddyPreview interface {
	UpsertRoute(ctx context.Context, cfg model.CaddyRouteConfig) error
	DeleteRoute(ctx context.Context, hostname string) error
	AutomateCert(ctx context.Context, hostname string) error
}

// Manager handles preview environment creation, expiry, and cleanup.
type Manager struct {
	repo          *store.Repository
	caddy         CaddyPreview
	previewDomain string // base domain for generated preview hostnames, e.g. "preview.agnivo.app"
	ttl           time.Duration
	log           *zap.Logger
}

// NewManager constructs a preview Manager.
func NewManager(repo *store.Repository, caddy CaddyPreview, previewDomain string, ttl time.Duration, log *zap.Logger) *Manager {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	if previewDomain == "" {
		previewDomain = "preview.agnivo.app"
	}
	return &Manager{
		repo:          repo,
		caddy:         caddy,
		previewDomain: previewDomain,
		ttl:           ttl,
		log:           log,
	}
}

// CreateInput is the payload for creating a preview environment.
type CreateInput struct {
	OrgID          string
	ProjectID      string
	DeploymentID   string
	Upstream       string
	Branch         string
	CommitSHA      string
	CustomHostname string // optional: override the auto-generated hostname
	TTL            time.Duration
	CorrelationID  string
}

// Create registers a preview deployment with a unique auto-generated hostname.
func (m *Manager) Create(ctx context.Context, in CreateInput) (model.Preview, error) {
	if in.Upstream == "" {
		return model.Preview{}, errors.New(errors.CodeInvalidArgument, "preview: upstream required")
	}
	if in.DeploymentID == "" {
		return model.Preview{}, errors.New(errors.CodeInvalidArgument, "preview: deployment_id required")
	}

	hostname := in.CustomHostname
	if hostname == "" {
		hostname = m.generateHostname(in.Branch, in.ProjectID)
	}

	ttl := in.TTL
	if ttl <= 0 {
		ttl = m.ttl
	}
	expiresAt := time.Now().UTC().Add(ttl)

	preview, err := m.repo.UpsertPreview(ctx, model.Preview{
		ID:            idx.NewUUID(),
		OrgID:         in.OrgID,
		ProjectID:     in.ProjectID,
		DeploymentID:  in.DeploymentID,
		Hostname:      hostname,
		Upstream:      in.Upstream,
		Branch:        in.Branch,
		CommitSHA:     in.CommitSHA,
		Status:        model.PreviewStatusActive,
		ExpiresAt:     &expiresAt,
		CorrelationID: in.CorrelationID,
	})
	if err != nil {
		return model.Preview{}, err
	}

	// Register the route in Caddy.
	if err := m.caddy.UpsertRoute(ctx, model.CaddyRouteConfig{
		Hostname:       hostname,
		Upstream:       in.Upstream,
		TLSEnabled:     true,
		HTTPSRedirect:  true,
		TimeoutSeconds: 30,
		MaxRetries:     2,
	}); err != nil {
		m.log.Warn("preview: caddy upsert failed", zap.String("hostname", hostname), zap.Error(err))
	}

	// Request TLS for the preview subdomain.
	if err := m.caddy.AutomateCert(ctx, hostname); err != nil {
		m.log.Warn("preview: tls automate failed", zap.String("hostname", hostname), zap.Error(err))
	}

	m.log.Info("preview: created",
		zap.String("hostname", hostname),
		zap.String("deployment_id", in.DeploymentID),
		zap.Time("expires_at", expiresAt))
	return preview, nil
}

// Delete removes a preview environment and its route from Caddy.
func (m *Manager) Delete(ctx context.Context, deploymentID string) error {
	preview, err := m.repo.GetPreviewByDeployment(ctx, deploymentID)
	if err != nil {
		if errors.IsCode(err, errors.CodeNotFound) {
			return nil
		}
		return err
	}

	if err := m.caddy.DeleteRoute(ctx, preview.Hostname); err != nil {
		m.log.Warn("preview: caddy delete failed", zap.String("hostname", preview.Hostname), zap.Error(err))
	}

	if err := m.repo.SoftDeletePreview(ctx, preview.ID); err != nil {
		return err
	}

	m.log.Info("preview: deleted", zap.String("hostname", preview.Hostname))
	return nil
}

// CleanupExpired removes all previews that have passed their TTL.
func (m *Manager) CleanupExpired(ctx context.Context) (int, error) {
	previews, err := m.repo.ListExpiredPreviews(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, p := range previews {
		if err := m.caddy.DeleteRoute(ctx, p.Hostname); err != nil {
			m.log.Warn("preview: cleanup caddy delete failed",
				zap.String("hostname", p.Hostname), zap.Error(err))
		}
		if err := m.repo.SoftDeletePreview(ctx, p.ID); err != nil {
			m.log.Warn("preview: cleanup db delete failed",
				zap.String("id", p.ID), zap.Error(err))
			continue
		}
		removed++
	}
	if removed > 0 {
		m.log.Info("preview: cleanup complete", zap.Int("removed", removed))
	}
	return removed, nil
}

// Get returns the preview for a deployment.
func (m *Manager) Get(ctx context.Context, deploymentID string) (model.Preview, error) {
	return m.repo.GetPreviewByDeployment(ctx, deploymentID)
}

// List returns all active previews for a project.
func (m *Manager) List(ctx context.Context, orgID, projectID string) ([]model.Preview, error) {
	return m.repo.ListActivePreviewsByProject(ctx, orgID, projectID)
}

// generateHostname produces a deterministic, URL-safe preview subdomain from
// the branch name and a short project identifier.
func (m *Manager) generateHostname(branch, projectID string) string {
	branchSlug := slugify(branch)
	if branchSlug == "" {
		branchSlug = "preview"
	}
	// Use first 8 chars of project UUID for brevity.
	projShort := projectID
	if len(projShort) > 8 {
		projShort = projShort[:8]
	}
	return fmt.Sprintf("%s-%s.%s", branchSlug, projShort, m.previewDomain)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		} else if c == '/' || c == '_' || c == '.' {
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	// Trim trailing dashes.
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}
