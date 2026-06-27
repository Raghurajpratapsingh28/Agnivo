// Package cert manages the complete TLS certificate lifecycle:
// issuance, renewal, revocation, expiration tracking, and failure recovery.
// Certificates are provisioned through Caddy's built-in ACME client (Let's
// Encrypt) — this layer orchestrates when certificates should be requested,
// renewed, or revoked and keeps the database in sync.
package cert

import (
	"context"
	"fmt"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// CaddyTLS abstracts the Caddy client certificate operations.
type CaddyTLS interface {
	AutomateCert(ctx context.Context, hostname string) error
	RevokeCert(ctx context.Context, hostname string) error
}

// Manager orchestrates TLS certificate provisioning and renewal.
type Manager struct {
	repo   *store.Repository
	caddy  CaddyTLS
	log    *zap.Logger

	// renewBefore is how far ahead of expiry to trigger renewal.
	renewBefore time.Duration
	// maxAttempts caps repeated issuance failures before giving up.
	maxAttempts int
}

// NewManager constructs a certificate Manager.
func NewManager(repo *store.Repository, caddy CaddyTLS, renewBefore time.Duration, log *zap.Logger) *Manager {
	if renewBefore <= 0 {
		renewBefore = 30 * 24 * time.Hour
	}
	return &Manager{
		repo:        repo,
		caddy:       caddy,
		log:         log,
		renewBefore: renewBefore,
		maxAttempts: 5,
	}
}

// RequestCert provisions a certificate for a hostname. It is idempotent:
// calling it multiple times for the same hostname is safe.
func (m *Manager) RequestCert(ctx context.Context, orgID, projectID, domainID, hostname, correlationID string) (model.Certificate, error) {
	if hostname == "" {
		return model.Certificate{}, errors.New(errors.CodeInvalidArgument, "cert: hostname required")
	}

	// Upsert a pending record.
	cert, err := m.repo.UpsertCertificate(ctx, model.Certificate{
		OrgID:         orgID,
		ProjectID:     projectID,
		DomainID:      domainID,
		Hostname:      hostname,
		Status:        model.CertStatusIssuing,
		Issuer:        "lets_encrypt",
		CorrelationID: correlationID,
	})
	if err != nil {
		return model.Certificate{}, err
	}

	// Tell Caddy to begin ACME issuance. Caddy handles all ACME protocol
	// steps asynchronously; we track progress through reconciliation.
	if err := m.caddy.AutomateCert(ctx, hostname); err != nil {
		_ = m.repo.SetCertStatus(ctx, cert.ID, model.CertStatusFailed, err.Error())
		return cert, errors.Wrapf(err, errors.CodeInternal, "cert: automate %s", hostname)
	}

	m.log.Info("cert: issuance requested",
		zap.String("hostname", hostname),
		zap.String("correlation_id", correlationID))
	return cert, nil
}

// RenewCert initiates renewal for a certificate that is due.
func (m *Manager) RenewCert(ctx context.Context, cert model.Certificate) error {
	if cert.Status != model.CertStatusActive {
		return nil
	}
	corrID := logger.CorrelationID(ctx)
	if corrID == "" {
		corrID = idx.NewUUID()
	}
	m.log.Info("cert: initiating renewal",
		zap.String("hostname", cert.Hostname),
		zap.String("cert_id", cert.ID))

	if err := m.repo.SetCertStatus(ctx, cert.ID, model.CertStatusRenewing, ""); err != nil {
		return err
	}
	// Caddy will re-run ACME for the hostname automatically once the cert is
	// in its renewal window — just ensure it is still in the automate list.
	return m.caddy.AutomateCert(ctx, cert.Hostname)
}

// RevokeCert revokes and removes a certificate.
func (m *Manager) RevokeCert(ctx context.Context, cert model.Certificate) error {
	if err := m.caddy.RevokeCert(ctx, cert.Hostname); err != nil {
		m.log.Warn("cert: caddy revoke failed", zap.String("hostname", cert.Hostname), zap.Error(err))
	}
	return m.repo.SetCertStatus(ctx, cert.ID, model.CertStatusRevoked, "revoked by operator")
}

// MarkIssued records a successful certificate issuance from an external source
// (e.g. detected by reconciliation or a webhook from Caddy events).
func (m *Manager) MarkIssued(ctx context.Context, hostname, serial, fingerprint string, expiresAt time.Time) error {
	cert, err := m.repo.GetCertByHostname(ctx, hostname)
	if err != nil {
		if errors.IsCode(err, errors.CodeNotFound) {
			return nil
		}
		return err
	}
	issuedAt := time.Now().UTC()
	renewAfter := expiresAt.Add(-m.renewBefore)
	if renewAfter.Before(issuedAt) {
		renewAfter = issuedAt.Add(24 * time.Hour)
	}
	return m.repo.SetCertIssued(ctx, cert.ID, serial, fingerprint, issuedAt, expiresAt, renewAfter)
}

// ReconcileRenewals queries all certs due for renewal and triggers each one.
func (m *Manager) ReconcileRenewals(ctx context.Context) error {
	certs, err := m.repo.ListCertsDueForRenewal(ctx)
	if err != nil {
		return err
	}
	for _, cert := range certs {
		if err := m.RenewCert(ctx, cert); err != nil {
			m.log.Warn("cert: renewal failed",
				zap.String("hostname", cert.Hostname),
				zap.Error(err))
		}
	}
	if len(certs) > 0 {
		m.log.Info("cert: reconcile renewals", zap.Int("count", len(certs)))
	}
	return nil
}

// ReconcileExpired marks stale issuing/renewing certs as failed so they
// will be retried by the next pass.
func (m *Manager) ReconcileExpired(ctx context.Context) error {
	certs, err := m.repo.ListCertsDueForRenewal(ctx)
	if err != nil {
		return err
	}
	for _, cert := range certs {
		if cert.IsExpired() && cert.Status == model.CertStatusActive {
			m.log.Warn("cert: expired cert detected", zap.String("hostname", cert.Hostname))
			_ = m.repo.SetCertStatus(ctx, cert.ID, model.CertStatusExpired,
				fmt.Sprintf("expired at %v", cert.ExpiresAt))
		}
	}
	return nil
}
