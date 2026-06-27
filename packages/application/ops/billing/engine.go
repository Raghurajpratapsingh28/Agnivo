// Package billing implements the billing engine: plan management, subscriptions,
// invoice generation, trials, credits, plan changes, and grace periods.
// All Stripe interactions are behind the BillingProvider interface so the
// implementation can be swapped without touching business logic.
package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// BillingProvider abstracts the external payment processor (Stripe, etc.).
// Implementations must be idempotent — the engine may call them multiple times.
type BillingProvider interface {
	// CreateCustomer creates or retrieves a customer record.
	CreateCustomer(ctx context.Context, orgID, email, name string) (customerID string, err error)
	// CreateSubscription creates a subscription for the customer.
	CreateSubscription(ctx context.Context, customerID, priceID string, trialDays int) (subID string, err error)
	// UpdateSubscription changes the price of an existing subscription.
	UpdateSubscription(ctx context.Context, subID, newPriceID string, prorate bool) error
	// CancelSubscription cancels a subscription at period end.
	CancelSubscription(ctx context.Context, subID string, immediately bool) error
	// CreateInvoice creates and finalises an invoice.
	CreateInvoice(ctx context.Context, customerID string, amountCents int64, description string) (invoiceID string, err error)
	// GetInvoiceStatus returns "paid", "open", "void", or "uncollectible".
	GetInvoiceStatus(ctx context.Context, invoiceID string) (string, error)
}

// NopProvider is a no-op billing provider (used in testing and free-tier).
type NopProvider struct{}

func (NopProvider) CreateCustomer(_ context.Context, orgID, _, _ string) (string, error) {
	s := orgID
	if len(s) > 8 {
		s = s[:8]
	}
	return "cus_nop_" + s, nil
}
func (NopProvider) CreateSubscription(_ context.Context, _, _ string, _ int) (string, error) {
	return "sub_nop_" + idx.NewUUID()[:8], nil
}
func (NopProvider) UpdateSubscription(_ context.Context, _, _ string, _ bool) error { return nil }
func (NopProvider) CancelSubscription(_ context.Context, _ string, _ bool) error    { return nil }
func (NopProvider) CreateInvoice(_ context.Context, _ string, _ int64, _ string) (string, error) {
	return "inv_nop_" + idx.NewUUID()[:8], nil
}
func (NopProvider) GetInvoiceStatus(_ context.Context, _ string) (string, error) { return "paid", nil }

// Engine orchestrates the billing lifecycle.
type Engine struct {
	repo     *store.Repository
	provider BillingProvider
	log      *zap.Logger
}

// NewEngine constructs a billing Engine.
func NewEngine(repo *store.Repository, provider BillingProvider, log *zap.Logger) *Engine {
	if provider == nil {
		provider = NopProvider{}
	}
	return &Engine{repo: repo, provider: provider, log: log}
}

// Subscribe activates a paid plan subscription for an org.
// trialDays = 0 means no trial.
func (e *Engine) Subscribe(ctx context.Context, orgID, email, name string, planID model.PlanID, interval model.BillingInterval, trialDays int, correlationID string) (model.Subscription, error) {
	if correlationID == "" {
		correlationID = logger.CorrelationID(ctx)
	}

	customerID, err := e.provider.CreateCustomer(ctx, orgID, email, name)
	if err != nil {
		return model.Subscription{}, errors.Wrapf(err, errors.CodeInternal, "billing: create customer for %s", orgID)
	}

	priceID := e.priceIDFor(planID, interval)
	subID, err := e.provider.CreateSubscription(ctx, customerID, priceID, trialDays)
	if err != nil {
		return model.Subscription{}, errors.Wrapf(err, errors.CodeInternal, "billing: create subscription for %s", orgID)
	}

	now := time.Now().UTC()
	status := model.SubStatusActive
	var trialEndsAt *time.Time
	if trialDays > 0 {
		status = model.SubStatusTrialing
		t := now.Add(time.Duration(trialDays) * 24 * time.Hour)
		trialEndsAt = &t
	}
	periodEnd := e.nextPeriodEnd(now, interval)

	sub, err := e.repo.UpsertSubscription(ctx, model.Subscription{
		OrgID:              orgID,
		PlanID:             planID,
		Status:             status,
		Interval:           interval,
		StripeSubID:        subID,
		StripeCustomerID:   customerID,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		TrialEndsAt:        trialEndsAt,
		CorrelationID:      correlationID,
	})
	if err != nil {
		return model.Subscription{}, err
	}
	e.log.Info("billing: subscribed",
		zap.String("org_id", orgID),
		zap.String("plan_id", string(planID)),
		zap.Int("trial_days", trialDays))
	return sub, nil
}

// ChangePlan upgrades or downgrades a subscription.
func (e *Engine) ChangePlan(ctx context.Context, orgID string, newPlanID model.PlanID, correlationID string) (model.Subscription, error) {
	sub, err := e.repo.GetSubscription(ctx, orgID)
	if err != nil {
		// If no subscription exists, create one on the free plan first.
		sub, err = e.ensureFreeSubscription(ctx, orgID, correlationID)
		if err != nil {
			return model.Subscription{}, err
		}
	}

	if sub.StripeSubID != "" {
		priceID := e.priceIDFor(newPlanID, sub.Interval)
		if err := e.provider.UpdateSubscription(ctx, sub.StripeSubID, priceID, true); err != nil {
			return sub, errors.Wrapf(err, errors.CodeInternal, "billing: update subscription for %s", orgID)
		}
	}

	sub.PlanID = newPlanID
	sub.CorrelationID = correlationID
	sub, err = e.repo.UpsertSubscription(ctx, sub)
	if err != nil {
		return sub, err
	}
	e.log.Info("billing: plan changed",
		zap.String("org_id", orgID),
		zap.String("new_plan", string(newPlanID)))
	return sub, nil
}

// Cancel cancels a subscription. If graceDays > 0, the org retains access
// during a grace period before downgrading to free.
func (e *Engine) Cancel(ctx context.Context, orgID string, immediately bool, graceDays int, correlationID string) (model.Subscription, error) {
	sub, err := e.repo.GetSubscription(ctx, orgID)
	if err != nil {
		return model.Subscription{}, err
	}
	if sub.StripeSubID != "" {
		if err := e.provider.CancelSubscription(ctx, sub.StripeSubID, immediately); err != nil {
			return sub, errors.Wrapf(err, errors.CodeInternal, "billing: cancel subscription for %s", orgID)
		}
	}
	now := time.Now().UTC()
	sub.CanceledAt = &now
	sub.CorrelationID = correlationID
	if graceDays > 0 {
		sub.Status = model.SubStatusGracePeriod
		graceEnd := now.Add(time.Duration(graceDays) * 24 * time.Hour)
		sub.GracePeriodEndsAt = &graceEnd
	} else {
		sub.Status = model.SubStatusCanceled
	}
	return e.repo.UpsertSubscription(ctx, sub)
}

// AddCredit grants monetary credit to an org account.
func (e *Engine) AddCredit(ctx context.Context, orgID string, amountCents int64, reason, couponCode, correlationID string) (model.Credit, error) {
	if amountCents <= 0 {
		return model.Credit{}, errors.New(errors.CodeInvalidArgument, "billing: credit amount must be positive")
	}
	return e.repo.AddCredit(ctx, model.Credit{
		OrgID:         orgID,
		AmountCents:   amountCents,
		Reason:        reason,
		CouponCode:    couponCode,
		CorrelationID: correlationID,
	})
}

// GenerateInvoice produces a billing invoice for an org for the current period.
func (e *Engine) GenerateInvoice(ctx context.Context, orgID string, amountCents int64, correlationID string) (model.Invoice, error) {
	sub, err := e.repo.GetSubscription(ctx, orgID)
	if err != nil {
		return model.Invoice{}, err
	}

	credits, err := e.repo.AvailableCredits(ctx, orgID)
	if err != nil {
		credits = 0
	}
	applied := min64(credits, amountCents)
	total := amountCents - applied
	if total < 0 {
		total = 0
	}

	now := time.Now().UTC()
	due := now.Add(30 * 24 * time.Hour)
	invID := ""
	if sub.StripeCustomerID != "" && total > 0 {
		invID, err = e.provider.CreateInvoice(ctx, sub.StripeCustomerID, total,
			fmt.Sprintf("Agnivo usage %s – %s", sub.CurrentPeriodStart.Format("2006-01-02"), sub.CurrentPeriodEnd.Format("2006-01-02")))
		if err != nil {
			e.log.Warn("billing: create invoice failed", zap.String("org_id", orgID), zap.Error(err))
		}
	}

	return e.repo.InsertInvoice(ctx, model.Invoice{
		OrgID:          orgID,
		SubscriptionID: sub.ID,
		Status:         model.InvoicePending,
		PeriodStart:    sub.CurrentPeriodStart,
		PeriodEnd:      sub.CurrentPeriodEnd,
		AmountCents:    amountCents,
		CreditsCents:   applied,
		TotalCents:     total,
		Currency:       "usd",
		StripeInvID:    invID,
		DueAt:          &due,
		CorrelationID:  correlationID,
	})
}

// RenewSubscription advances the billing period for a subscription.
func (e *Engine) RenewSubscription(ctx context.Context, sub model.Subscription) (model.Subscription, error) {
	now := time.Now().UTC()
	sub.CurrentPeriodStart = now
	sub.CurrentPeriodEnd = e.nextPeriodEnd(now, sub.Interval)
	sub.Status = model.SubStatusActive
	return e.repo.UpsertSubscription(ctx, sub)
}

// ProcessGraceExpiry downgrades orgs whose grace period has elapsed.
func (e *Engine) ProcessGraceExpiry(ctx context.Context, sub model.Subscription, correlationID string) (model.Subscription, error) {
	sub.Status = model.SubStatusCanceled
	sub.PlanID = model.PlanFree
	sub.CorrelationID = correlationID
	return e.repo.UpsertSubscription(ctx, sub)
}

func (e *Engine) ensureFreeSubscription(ctx context.Context, orgID, correlationID string) (model.Subscription, error) {
	now := time.Now().UTC()
	return e.repo.UpsertSubscription(ctx, model.Subscription{
		OrgID:              orgID,
		PlanID:             model.PlanFree,
		Status:             model.SubStatusActive,
		Interval:           model.IntervalMonthly,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   e.nextPeriodEnd(now, model.IntervalMonthly),
		CorrelationID:      correlationID,
	})
}

func (e *Engine) nextPeriodEnd(from time.Time, interval model.BillingInterval) time.Time {
	switch interval {
	case model.IntervalYearly:
		return from.AddDate(1, 0, 0)
	default: // monthly
		return from.AddDate(0, 1, 0)
	}
}

func (e *Engine) priceIDFor(plan model.PlanID, interval model.BillingInterval) string {
	// In production these come from a configuration table; here we generate
	// deterministic test IDs so the nop provider never needs real Stripe prices.
	return fmt.Sprintf("price_%s_%s", plan, interval)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
