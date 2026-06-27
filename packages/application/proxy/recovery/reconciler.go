// Package recovery implements the proxy-manager reconciliation loop:
// it periodically compares the desired state (database) with the actual state
// (Caddy) and heals any drift. It also drives DNS verification retries,
// certificate renewal, and preview cleanup.
package recovery

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/cert"
	"github.com/agnivo/agnivo/packages/application/proxy/dns"
	"github.com/agnivo/agnivo/packages/application/proxy/events"
	proxmetrics "github.com/agnivo/agnivo/packages/application/proxy/metrics"
	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/preview"
	"github.com/agnivo/agnivo/packages/application/proxy/route"
	"github.com/agnivo/agnivo/packages/application/proxy/store"
	"github.com/agnivo/agnivo/packages/platform/config"
	"go.uber.org/zap"
)

// Reconciler drives all background healing loops.
type Reconciler struct {
	repo     *store.Repository
	engine   *route.Engine
	cert     *cert.Manager
	preview  *preview.Manager
	verifier *dns.Verifier
	pub      *events.Publisher
	metrics  *proxmetrics.Metrics
	cfg      config.ProxyManager
	log      *zap.Logger
}

// NewReconciler constructs a Reconciler.
func NewReconciler(
	repo *store.Repository,
	engine *route.Engine,
	certMgr *cert.Manager,
	prevMgr *preview.Manager,
	verifier *dns.Verifier,
	pub *events.Publisher,
	metrics *proxmetrics.Metrics,
	cfg config.ProxyManager,
	log *zap.Logger,
) *Reconciler {
	return &Reconciler{
		repo:     repo,
		engine:   engine,
		cert:     certMgr,
		preview:  prevMgr,
		verifier: verifier,
		pub:      pub,
		metrics:  metrics,
		cfg:      cfg,
		log:      log,
	}
}

// Run executes the reconciliation loop until ctx is canceled.
func (r *Reconciler) Run(ctx context.Context) error {
	interval := r.cfg.ReconcileInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run an immediate pass on startup to heal any state from a previous crash.
	r.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	start := time.Now()
	r.metrics.ReconcileRuns.Inc()

	var errs int

	if err := r.reconcileRoutes(ctx); err != nil {
		r.log.Warn("reconciler: routes failed", zap.Error(err))
		errs++
	}
	if err := r.reconcileVerifications(ctx); err != nil {
		r.log.Warn("reconciler: verifications failed", zap.Error(err))
		errs++
	}
	if err := r.reconcileCerts(ctx); err != nil {
		r.log.Warn("reconciler: certs failed", zap.Error(err))
		errs++
	}
	if err := r.reconcilePreviews(ctx); err != nil {
		r.log.Warn("reconciler: previews failed", zap.Error(err))
		errs++
	}

	ms := float64(time.Since(start).Milliseconds())
	r.metrics.ReconcileMs.Observe(ms)
	if errs > 0 {
		r.metrics.ReconcileErrors.Add(float64(errs))
	}

	r.log.Debug("reconciler: pass complete",
		zap.Float64("ms", ms),
		zap.Int("errors", errs))
}

// reconcileRoutes ensures all active DB routes are present in Caddy.
func (r *Reconciler) reconcileRoutes(ctx context.Context) error {
	if err := r.engine.ReconcileAll(ctx); err != nil {
		return err
	}
	_ = r.pub.PublishAsync(ctx, events.ReconcileCompleted, events.Meta{}, map[string]any{
		"subsystem": "routes",
	})
	return nil
}

// reconcileVerifications retries pending DNS verifications.
func (r *Reconciler) reconcileVerifications(ctx context.Context) error {
	verifications, err := r.repo.ListPendingVerifications(ctx)
	if err != nil {
		return err
	}
	for _, v := range verifications {
		result := r.verifier.Verify(ctx, v)
		if result.Verified {
			if err := r.repo.MarkVerified(ctx, v.ID); err != nil {
				r.log.Warn("reconciler: mark verified failed", zap.String("domain", v.Hostname), zap.Error(err))
				continue
			}
			_ = r.pub.PublishAsync(ctx, events.DomainVerified, events.Meta{
				OrgID:         v.OrgID,
				ProjectID:     v.ProjectID,
				DomainID:      v.DomainID,
				CorrelationID: v.CorrelationID,
			}, map[string]string{"hostname": v.Hostname})
		} else {
			if err := r.repo.MarkVerificationFailed(ctx, v.ID, result.Reason); err != nil {
				r.log.Warn("reconciler: mark verify failed", zap.String("domain", v.Hostname), zap.Error(err))
			}
			_ = r.pub.PublishAsync(ctx, events.DomainVerifyFailed, events.Meta{
				OrgID:     v.OrgID,
				DomainID:  v.DomainID,
			}, map[string]string{"hostname": v.Hostname, "reason": result.Reason})
		}
	}
	return nil
}

// reconcileCerts triggers renewal for certs due and marks expired ones.
func (r *Reconciler) reconcileCerts(ctx context.Context) error {
	if err := r.cert.ReconcileRenewals(ctx); err != nil {
		return err
	}
	return r.cert.ReconcileExpired(ctx)
}

// reconcilePreviews cleans up expired preview environments.
func (r *Reconciler) reconcilePreviews(ctx context.Context) error {
	removed, err := r.preview.CleanupExpired(ctx)
	if err != nil {
		return err
	}
	for i := 0; i < removed; i++ {
		r.metrics.PreviewExpired.Inc()
	}
	return nil
}

// DomainVerifyRequest processes a single on-demand domain verification job.
func (r *Reconciler) DomainVerifyRequest(ctx context.Context, orgID, projectID, domainID, hostname, method, correlationID string) error {
	challenge := dns.GenerateChallenge(dns.VerificationMethod(method), domainID)
	_, err := r.repo.UpsertVerification(ctx, model.DomainVerification{
		OrgID:          orgID,
		ProjectID:      projectID,
		DomainID:       domainID,
		Hostname:       hostname,
		Method:         method,
		ChallengeValue: challenge,
		Status:         "pending",
		CorrelationID:  correlationID,
	})
	return err
}

// SSLRequest processes a single on-demand SSL provisioning job.
func (r *Reconciler) SSLRequest(ctx context.Context, orgID, projectID, domainID, hostname, correlationID string) error {
	_, err := r.cert.RequestCert(ctx, orgID, projectID, domainID, hostname, correlationID)
	return err
}
