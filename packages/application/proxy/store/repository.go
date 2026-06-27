package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists all proxy-manager state.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a proxy repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// ──────────────────────────────── Routes ─────────────────────────────────────

// UpsertRoute idempotently creates or updates a route record.
func (r *Repository) UpsertRoute(ctx context.Context, rt model.Route) (model.Route, error) {
	if rt.ID == "" {
		rt.ID = idx.NewUUID()
	}
	if rt.Metadata == nil {
		rt.Metadata, _ = json.Marshal(map[string]any{})
	}
	if rt.AddHeaders == nil {
		rt.AddHeaders, _ = json.Marshal(map[string]string{})
	}
	const q = `
INSERT INTO proxy_routes
	(id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
	 traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
	 strip_prefix, add_headers, timeout_seconds, max_retries, version,
	 correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,now(),now())
ON CONFLICT (hostname) WHERE deleted_at IS NULL DO UPDATE SET
	deployment_id=EXCLUDED.deployment_id, upstream=EXCLUDED.upstream, status=EXCLUDED.status,
	traffic_mode=EXCLUDED.traffic_mode, canary_weight=EXCLUDED.canary_weight,
	previous_upstream=EXCLUDED.previous_upstream, tls_enabled=EXCLUDED.tls_enabled,
	https_redirect=EXCLUDED.https_redirect, strip_prefix=EXCLUDED.strip_prefix,
	add_headers=EXCLUDED.add_headers, timeout_seconds=EXCLUDED.timeout_seconds,
	max_retries=EXCLUDED.max_retries, version=proxy_routes.version+1,
	correlation_id=EXCLUDED.correlation_id, metadata=EXCLUDED.metadata, updated_at=now()
RETURNING id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
	traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
	strip_prefix, add_headers, timeout_seconds, max_retries, version,
	correlation_id, metadata, created_at, updated_at, deleted_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		rt.ID, rt.OrgID, rt.ProjectID, rt.DeploymentID, rt.DomainID, rt.Hostname, rt.Upstream,
		rt.Kind, rt.Status, rt.TrafficMode, rt.CanaryWeight, rt.PreviousUpstream,
		rt.TLSEnabled, rt.HTTPSRedirect, rt.StripPrefix, rt.AddHeaders,
		rt.TimeoutSeconds, rt.MaxRetries, rt.Version, rt.CorrelationID, rt.Metadata)
	return scanRoute(row)
}

// GetRouteByHostname returns the active route for a hostname.
func (r *Repository) GetRouteByHostname(ctx context.Context, hostname string) (model.Route, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
		traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
		strip_prefix, add_headers, timeout_seconds, max_retries, version,
		correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_routes WHERE hostname=$1 AND deleted_at IS NULL`
	return scanRoute(r.db.Conn(ctx).QueryRow(ctx, q, hostname))
}

// GetRouteByDeployment returns the active route for a deployment.
func (r *Repository) GetRouteByDeployment(ctx context.Context, deploymentID string) (model.Route, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
		traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
		strip_prefix, add_headers, timeout_seconds, max_retries, version,
		correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_routes WHERE deployment_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`
	return scanRoute(r.db.Conn(ctx).QueryRow(ctx, q, deploymentID))
}

// ListRoutesByProject returns all active routes for a project.
func (r *Repository) ListRoutesByProject(ctx context.Context, orgID, projectID string) ([]model.Route, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
		traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
		strip_prefix, add_headers, timeout_seconds, max_retries, version,
		correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_routes WHERE org_id=$1 AND project_id=$2 AND deleted_at IS NULL ORDER BY hostname`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list routes")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Route])
}

// ListActiveRoutes returns all routes in active status (for reconciliation).
func (r *Repository) ListActiveRoutes(ctx context.Context) ([]model.Route, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, domain_id, hostname, upstream, kind, status,
		traffic_mode, canary_weight, previous_upstream, tls_enabled, https_redirect,
		strip_prefix, add_headers, timeout_seconds, max_retries, version,
		correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_routes WHERE status IN ('active','draining') AND deleted_at IS NULL ORDER BY hostname`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list active routes")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Route])
}

// SetRouteStatus updates status for a single route.
func (r *Repository) SetRouteStatus(ctx context.Context, id string, status model.RouteStatus) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE proxy_routes SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return postgres.Translate(err, "proxy: set route status")
}

// UpdateRouteTraffic atomically switches upstream and traffic mode.
func (r *Repository) UpdateRouteTraffic(ctx context.Context, id, upstream, previous string, mode model.TrafficMode, weight int) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE proxy_routes SET
	upstream=$2, previous_upstream=$3, traffic_mode=$4, canary_weight=$5,
	version=version+1, updated_at=now()
WHERE id=$1`, id, upstream, previous, mode, weight)
	return postgres.Translate(err, "proxy: update route traffic")
}

// SoftDeleteRoute marks a route as deleted and sets status removed.
func (r *Repository) SoftDeleteRoute(ctx context.Context, id string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE proxy_routes SET status='removed', deleted_at=now(), updated_at=now() WHERE id=$1`, id)
	return postgres.Translate(err, "proxy: delete route")
}

// RecordRouteVersion snapshots the current route state for rollback.
func (r *Repository) RecordRouteVersion(ctx context.Context, rt model.Route) error {
	meta, _ := json.Marshal(map[string]any{})
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO proxy_route_versions (id, route_id, org_id, project_id, deployment_id,
	hostname, upstream, version, correlation_id, metadata, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())`,
		idx.NewUUID(), rt.ID, rt.OrgID, rt.ProjectID, rt.DeploymentID,
		rt.Hostname, rt.Upstream, rt.Version, rt.CorrelationID, meta)
	return postgres.Translate(err, "proxy: record route version")
}

// ListRouteVersions returns version history for a route.
func (r *Repository) ListRouteVersions(ctx context.Context, routeID string, limit int) ([]model.RouteVersion, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, route_id, org_id, project_id, deployment_id,
	hostname, upstream, version, correlation_id, metadata, created_at
FROM proxy_route_versions WHERE route_id=$1 ORDER BY version DESC LIMIT $2`,
		routeID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list route versions")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.RouteVersion])
}

// ──────────────────────────────── Certificates ───────────────────────────────

// UpsertCertificate idempotently creates or updates a certificate record.
func (r *Repository) UpsertCertificate(ctx context.Context, c model.Certificate) (model.Certificate, error) {
	if c.ID == "" {
		c.ID = idx.NewUUID()
	}
	if c.Metadata == nil {
		c.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO proxy_certificates
	(id, org_id, project_id, domain_id, hostname, status, issuer, serial_number, fingerprint,
	 issued_at, expires_at, renew_after, is_wildcard, acme_challenge, failure_reason,
	 attempts, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now(),now())
ON CONFLICT (hostname) DO UPDATE SET
	status=EXCLUDED.status, issuer=EXCLUDED.issuer, serial_number=EXCLUDED.serial_number,
	fingerprint=EXCLUDED.fingerprint, issued_at=EXCLUDED.issued_at, expires_at=EXCLUDED.expires_at,
	renew_after=EXCLUDED.renew_after, acme_challenge=EXCLUDED.acme_challenge,
	failure_reason=EXCLUDED.failure_reason, attempts=EXCLUDED.attempts,
	correlation_id=EXCLUDED.correlation_id, metadata=EXCLUDED.metadata, updated_at=now()
RETURNING id, org_id, project_id, domain_id, hostname, status, issuer, serial_number, fingerprint,
	issued_at, expires_at, renew_after, is_wildcard, acme_challenge, failure_reason,
	attempts, correlation_id, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		c.ID, c.OrgID, c.ProjectID, c.DomainID, c.Hostname, c.Status, c.Issuer,
		c.SerialNumber, c.Fingerprint, c.IssuedAt, c.ExpiresAt, c.RenewAfter,
		c.IsWildcard, c.AcmeChallenge, c.FailureReason,
		c.Attempts, c.CorrelationID, c.Metadata)
	return scanCert(row)
}

// GetCertByHostname returns a certificate record by hostname.
func (r *Repository) GetCertByHostname(ctx context.Context, hostname string) (model.Certificate, error) {
	const q = `SELECT id, org_id, project_id, domain_id, hostname, status, issuer, serial_number, fingerprint,
		issued_at, expires_at, renew_after, is_wildcard, acme_challenge, failure_reason,
		attempts, correlation_id, metadata, created_at, updated_at
		FROM proxy_certificates WHERE hostname=$1`
	return scanCert(r.db.Conn(ctx).QueryRow(ctx, q, hostname))
}

// ListCertsDueForRenewal returns active certs where renew_after is past.
func (r *Repository) ListCertsDueForRenewal(ctx context.Context) ([]model.Certificate, error) {
	const q = `SELECT id, org_id, project_id, domain_id, hostname, status, issuer, serial_number, fingerprint,
		issued_at, expires_at, renew_after, is_wildcard, acme_challenge, failure_reason,
		attempts, correlation_id, metadata, created_at, updated_at
		FROM proxy_certificates WHERE status='active' AND renew_after <= now() ORDER BY renew_after`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list renewal certs")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Certificate])
}

// SetCertStatus updates certificate status.
func (r *Repository) SetCertStatus(ctx context.Context, id string, status model.CertStatus, reason string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE proxy_certificates SET status=$2, failure_reason=$3, attempts=attempts+1, updated_at=now() WHERE id=$1`,
		id, status, reason)
	return postgres.Translate(err, "proxy: set cert status")
}

// SetCertIssued records successful certificate issuance.
func (r *Repository) SetCertIssued(ctx context.Context, id, serial, fingerprint string, issuedAt, expiresAt, renewAfter time.Time) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE proxy_certificates SET
	status='active', serial_number=$2, fingerprint=$3,
	issued_at=$4, expires_at=$5, renew_after=$6,
	failure_reason='', updated_at=now()
WHERE id=$1`, id, serial, fingerprint, issuedAt, expiresAt, renewAfter)
	return postgres.Translate(err, "proxy: set cert issued")
}

// ──────────────────────────────── Verifications ──────────────────────────────

// UpsertVerification creates or updates a domain verification record.
func (r *Repository) UpsertVerification(ctx context.Context, v model.DomainVerification) (model.DomainVerification, error) {
	if v.ID == "" {
		v.ID = idx.NewUUID()
	}
	if v.Metadata == nil {
		v.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO proxy_domain_verifications
	(id, org_id, project_id, domain_id, hostname, method, challenge_value, status,
	 attempts, last_attempt_at, verified_at, expires_at, failure_reason,
	 correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
ON CONFLICT (domain_id) DO UPDATE SET
	method=EXCLUDED.method, challenge_value=EXCLUDED.challenge_value, status=EXCLUDED.status,
	attempts=EXCLUDED.attempts, last_attempt_at=EXCLUDED.last_attempt_at,
	verified_at=EXCLUDED.verified_at, expires_at=EXCLUDED.expires_at,
	failure_reason=EXCLUDED.failure_reason, correlation_id=EXCLUDED.correlation_id,
	metadata=EXCLUDED.metadata, updated_at=now()
RETURNING id, org_id, project_id, domain_id, hostname, method, challenge_value, status,
	attempts, last_attempt_at, verified_at, expires_at, failure_reason,
	correlation_id, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		v.ID, v.OrgID, v.ProjectID, v.DomainID, v.Hostname, v.Method, v.ChallengeValue,
		v.Status, v.Attempts, v.LastAttemptAt, v.VerifiedAt, v.ExpiresAt,
		v.FailureReason, v.CorrelationID, v.Metadata)
	return scanVerification(row)
}

// GetVerificationByDomain returns the verification record for a domain ID.
func (r *Repository) GetVerificationByDomain(ctx context.Context, domainID string) (model.DomainVerification, error) {
	const q = `SELECT id, org_id, project_id, domain_id, hostname, method, challenge_value, status,
		attempts, last_attempt_at, verified_at, expires_at, failure_reason,
		correlation_id, metadata, created_at, updated_at
		FROM proxy_domain_verifications WHERE domain_id=$1`
	return scanVerification(r.db.Conn(ctx).QueryRow(ctx, q, domainID))
}

// ListPendingVerifications returns verifications due for another attempt.
func (r *Repository) ListPendingVerifications(ctx context.Context) ([]model.DomainVerification, error) {
	const q = `SELECT id, org_id, project_id, domain_id, hostname, method, challenge_value, status,
		attempts, last_attempt_at, verified_at, expires_at, failure_reason,
		correlation_id, metadata, created_at, updated_at
		FROM proxy_domain_verifications
		WHERE status='pending' AND (last_attempt_at IS NULL OR last_attempt_at < now() - INTERVAL '2 minutes')
		ORDER BY attempts ASC, created_at ASC
		LIMIT 50`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list pending verifications")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.DomainVerification])
}

// MarkVerified updates a domain verification to verified.
func (r *Repository) MarkVerified(ctx context.Context, id string) error {
	now := time.Now().UTC()
	exp := now.Add(90 * 24 * time.Hour) // verification valid for 90 days
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE proxy_domain_verifications SET
	status='verified', verified_at=$2, expires_at=$3, failure_reason='', updated_at=now()
WHERE id=$1`, id, now, exp)
	return postgres.Translate(err, "proxy: mark verified")
}

// MarkVerificationFailed records a failed attempt.
func (r *Repository) MarkVerificationFailed(ctx context.Context, id, reason string) error {
	now := time.Now().UTC()
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE proxy_domain_verifications SET
	status='pending', last_attempt_at=$2, attempts=attempts+1, failure_reason=$3, updated_at=now()
WHERE id=$1`, id, now, reason)
	return postgres.Translate(err, "proxy: mark verification failed")
}

// ──────────────────────────────── Previews ───────────────────────────────────

// UpsertPreview creates or updates a preview environment record.
func (r *Repository) UpsertPreview(ctx context.Context, p model.Preview) (model.Preview, error) {
	if p.ID == "" {
		p.ID = idx.NewUUID()
	}
	if p.Metadata == nil {
		p.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO proxy_previews
	(id, org_id, project_id, deployment_id, hostname, upstream, branch, commit_sha,
	 status, expires_at, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),now())
ON CONFLICT (deployment_id) WHERE deleted_at IS NULL DO UPDATE SET
	hostname=EXCLUDED.hostname, upstream=EXCLUDED.upstream, branch=EXCLUDED.branch,
	commit_sha=EXCLUDED.commit_sha, status=EXCLUDED.status, expires_at=EXCLUDED.expires_at,
	correlation_id=EXCLUDED.correlation_id, metadata=EXCLUDED.metadata, updated_at=now()
RETURNING id, org_id, project_id, deployment_id, hostname, upstream, branch, commit_sha,
	status, expires_at, correlation_id, metadata, created_at, updated_at, deleted_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		p.ID, p.OrgID, p.ProjectID, p.DeploymentID, p.Hostname, p.Upstream,
		p.Branch, p.CommitSHA, p.Status, p.ExpiresAt, p.CorrelationID, p.Metadata)
	return scanPreview(row)
}

// GetPreviewByDeployment returns the preview for a deployment.
func (r *Repository) GetPreviewByDeployment(ctx context.Context, deploymentID string) (model.Preview, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, hostname, upstream, branch, commit_sha,
		status, expires_at, correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_previews WHERE deployment_id=$1 AND deleted_at IS NULL`
	return scanPreview(r.db.Conn(ctx).QueryRow(ctx, q, deploymentID))
}

// ListActivePreviewsByProject returns active previews for a project.
func (r *Repository) ListActivePreviewsByProject(ctx context.Context, orgID, projectID string) ([]model.Preview, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, hostname, upstream, branch, commit_sha,
		status, expires_at, correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_previews WHERE org_id=$1 AND project_id=$2 AND status='active' AND deleted_at IS NULL ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list previews")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Preview])
}

// ListExpiredPreviews returns previews that have passed their TTL.
func (r *Repository) ListExpiredPreviews(ctx context.Context) ([]model.Preview, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, hostname, upstream, branch, commit_sha,
		status, expires_at, correlation_id, metadata, created_at, updated_at, deleted_at
		FROM proxy_previews WHERE status='active' AND expires_at <= now() AND deleted_at IS NULL`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list expired previews")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Preview])
}

// SoftDeletePreview marks a preview as deleted.
func (r *Repository) SoftDeletePreview(ctx context.Context, id string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE proxy_previews SET status='deleted', deleted_at=now(), updated_at=now() WHERE id=$1`, id)
	return postgres.Translate(err, "proxy: delete preview")
}

// ──────────────────────────────── Events ─────────────────────────────────────

// RecordEvent persists a proxy audit event.
func (r *Repository) RecordEvent(ctx context.Context, e model.ProxyEvent) error {
	if e.ID == "" {
		e.ID = idx.NewUUID()
	}
	if e.Metadata == nil {
		e.Metadata, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO proxy_events
	(id, event_type, org_id, project_id, deployment_id, domain_id, route_id,
	 correlation_id, metadata, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`,
		e.ID, e.EventType, e.OrgID, e.ProjectID, e.DeploymentID,
		e.DomainID, e.RouteID, e.CorrelationID, e.Metadata)
	return postgres.Translate(err, "proxy: record event")
}

// ListEventsByDeployment returns recent events for a deployment.
func (r *Repository) ListEventsByDeployment(ctx context.Context, deploymentID string, limit int) ([]model.ProxyEvent, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, event_type, org_id, project_id, deployment_id, domain_id, route_id,
	correlation_id, metadata, created_at
FROM proxy_events WHERE deployment_id=$1 ORDER BY created_at DESC LIMIT $2`,
		deploymentID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "proxy: list events")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.ProxyEvent])
}

// ──────────────────────────────── Statistics ─────────────────────────────────

// Stats returns aggregate proxy statistics.
func (r *Repository) Stats(ctx context.Context) (model.ProxyMetrics, error) {
	var m model.ProxyMetrics
	err := r.db.Conn(ctx).QueryRow(ctx, `
SELECT
	count(*) FILTER (WHERE deleted_at IS NULL) AS route_count,
	count(*) FILTER (WHERE status='active' AND deleted_at IS NULL) AS active_routes
FROM proxy_routes`).Scan(&m.RouteCount, &m.ActiveRoutes)
	if err != nil {
		return m, postgres.Translate(err, "proxy: route stats")
	}
	err = r.db.Conn(ctx).QueryRow(ctx, `
SELECT
	count(*) AS cert_count,
	count(*) FILTER (WHERE status='active') AS active_certs,
	count(*) FILTER (WHERE status='active' AND renew_after <= now()) AS renewals_pending
FROM proxy_certificates`).Scan(&m.CertCount, &m.ActiveCerts, &m.RenewalsPending)
	if err != nil {
		return m, postgres.Translate(err, "proxy: cert stats")
	}
	err = r.db.Conn(ctx).QueryRow(ctx, `
SELECT count(*) FROM proxy_domain_verifications WHERE status='pending'`).Scan(&m.VerificationsPending)
	if err != nil {
		return m, postgres.Translate(err, "proxy: verify stats")
	}
	err = r.db.Conn(ctx).QueryRow(ctx, `
SELECT count(*) FROM proxy_previews WHERE status='active' AND deleted_at IS NULL`).Scan(&m.PreviewCount)
	if err != nil {
		return m, postgres.Translate(err, "proxy: preview stats")
	}
	return m, nil
}

// ─────────────────────────────── Scan helpers ────────────────────────────────

func scanRoute(row pgx.Row) (model.Route, error) {
	var rt model.Route
	err := row.Scan(
		&rt.ID, &rt.OrgID, &rt.ProjectID, &rt.DeploymentID, &rt.DomainID, &rt.Hostname,
		&rt.Upstream, &rt.Kind, &rt.Status, &rt.TrafficMode, &rt.CanaryWeight,
		&rt.PreviousUpstream, &rt.TLSEnabled, &rt.HTTPSRedirect, &rt.StripPrefix,
		&rt.AddHeaders, &rt.TimeoutSeconds, &rt.MaxRetries, &rt.Version,
		&rt.CorrelationID, &rt.Metadata, &rt.CreatedAt, &rt.UpdatedAt, &rt.DeletedAt,
	)
	if err != nil {
		return model.Route{}, postgres.Translate(err, "proxy: scan route")
	}
	return rt, nil
}

func scanCert(row pgx.Row) (model.Certificate, error) {
	var c model.Certificate
	err := row.Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.DomainID, &c.Hostname, &c.Status, &c.Issuer,
		&c.SerialNumber, &c.Fingerprint, &c.IssuedAt, &c.ExpiresAt, &c.RenewAfter,
		&c.IsWildcard, &c.AcmeChallenge, &c.FailureReason,
		&c.Attempts, &c.CorrelationID, &c.Metadata, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return model.Certificate{}, postgres.Translate(err, "proxy: scan cert")
	}
	return c, nil
}

func scanVerification(row pgx.Row) (model.DomainVerification, error) {
	var v model.DomainVerification
	err := row.Scan(
		&v.ID, &v.OrgID, &v.ProjectID, &v.DomainID, &v.Hostname, &v.Method, &v.ChallengeValue,
		&v.Status, &v.Attempts, &v.LastAttemptAt, &v.VerifiedAt, &v.ExpiresAt,
		&v.FailureReason, &v.CorrelationID, &v.Metadata, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return model.DomainVerification{}, postgres.Translate(err, "proxy: scan verification")
	}
	return v, nil
}

func scanPreview(row pgx.Row) (model.Preview, error) {
	var p model.Preview
	err := row.Scan(
		&p.ID, &p.OrgID, &p.ProjectID, &p.DeploymentID, &p.Hostname, &p.Upstream,
		&p.Branch, &p.CommitSHA, &p.Status, &p.ExpiresAt,
		&p.CorrelationID, &p.Metadata, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return model.Preview{}, postgres.Translate(err, "proxy: scan preview")
	}
	return p, nil
}
