// Package notification delivers notifications across multiple channels:
// email (SMTP), Slack (webhooks), Discord (webhooks), generic webhooks, and in-app.
// All channel implementations satisfy the Sender interface so they are trivially
// swappable and testable.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// Sender is the interface every channel transport must implement.
type Sender interface {
	Send(ctx context.Context, n model.Notification) error
}

// SMTPConfig configures the email transport.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// smtpSender delivers email via SMTP.
type smtpSender struct{ cfg SMTPConfig }

func (s *smtpSender) Send(_ context.Context, n model.Notification) error {
	if s.cfg.Host == "" {
		return errors.New(errors.CodeFailedPrecond, "notification: SMTP host not configured")
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	body := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", n.Recipient, n.Subject, n.Body)
	return smtp.SendMail(addr, auth, s.cfg.From, []string{n.Recipient}, []byte(body))
}

// webhookSender POSTs JSON to a URL (used for Slack, Discord, and generic webhooks).
type webhookSender struct {
	httpClient *http.Client
}

func (w *webhookSender) Send(ctx context.Context, n model.Notification) error {
	if n.Recipient == "" {
		return errors.New(errors.CodeInvalidArgument, "notification: webhook URL (recipient) required")
	}
	payload := map[string]string{
		"text":       n.Body,
		"event_type": n.EventType,
		"org_id":     n.OrgID,
	}
	// Slack/Discord expect "text" at the top level; generic webhooks get the full struct.
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "notification: marshal webhook")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.Recipient, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "notification: build webhook request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Agnivo-Notifications/1.0")
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "notification: webhook request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errors.New(errors.CodeInternal, fmt.Sprintf("notification: webhook returned %d", resp.StatusCode))
	}
	return nil
}

// nopSender discards notifications (used for in-app, which are already persisted).
type nopSender struct{}

func (nopSender) Send(_ context.Context, _ model.Notification) error { return nil }

// Dispatcher orchestrates notification delivery: it resolves the sender per
// channel, calls it, and updates the delivery status in the database.
type Dispatcher struct {
	repo    *store.Repository
	senders map[model.NotificationChannel]Sender
	log     *zap.Logger
}

// NewDispatcher constructs a Dispatcher with the provided channel transports.
func NewDispatcher(repo *store.Repository, smtpCfg SMTPConfig, log *zap.Logger) *Dispatcher {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	ws := &webhookSender{httpClient: httpClient}
	return &Dispatcher{
		repo: repo,
		senders: map[model.NotificationChannel]Sender{
			model.ChannelEmail:   &smtpSender{cfg: smtpCfg},
			model.ChannelSlack:   ws,
			model.ChannelDiscord: ws,
			model.ChannelWebhook: ws,
			model.ChannelInApp:   nopSender{},
		},
		log: log,
	}
}

// Enqueue persists a notification for async delivery and returns it.
func (d *Dispatcher) Enqueue(ctx context.Context, n model.Notification) (model.Notification, error) {
	return d.repo.InsertNotification(ctx, n)
}

// Deliver attempts to deliver a single notification by ID.
// It is idempotent: delivering an already-delivered notification is a no-op.
func (d *Dispatcher) Deliver(ctx context.Context, notificationID string) error {
	n, err := d.repo.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if n.Status == model.NotifDelivered {
		return nil
	}

	sender, ok := d.senders[n.Channel]
	if !ok {
		return errors.Newf(errors.CodeFailedPrecond, "notification: no sender for channel %s", n.Channel)
	}

	if err := sender.Send(ctx, n); err != nil {
		d.log.Warn("notification: delivery failed",
			zap.String("id", n.ID),
			zap.String("channel", string(n.Channel)),
			zap.Error(err))
		return d.repo.MarkNotificationFailed(ctx, n.ID, err.Error())
	}

	if err := d.repo.MarkNotificationDelivered(ctx, n.ID); err != nil {
		return err
	}
	d.log.Info("notification: delivered",
		zap.String("id", n.ID),
		zap.String("channel", string(n.Channel)),
		zap.String("event_type", n.EventType))
	return nil
}

// DrainPending delivers all pending notifications up to limit.
func (d *Dispatcher) DrainPending(ctx context.Context, limit int) (int, int, error) {
	pending, err := d.repo.ListPendingNotifications(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	delivered, failed := 0, 0
	for _, n := range pending {
		if err := d.Deliver(ctx, n.ID); err != nil {
			failed++
		} else {
			delivered++
		}
	}
	return delivered, failed, nil
}

// Build helpers for common notification types.

func DeploymentSuccess(orgID, userID, projectID, deploymentID, recipient string, corrID string) model.Notification {
	return model.Notification{
		OrgID:         orgID,
		UserID:        userID,
		ProjectID:     projectID,
		Channel:       model.ChannelEmail,
		EventType:     "deployment.success",
		Subject:       "Deployment succeeded",
		Body:          fmt.Sprintf("Deployment %s completed successfully.", deploymentID),
		Recipient:     recipient,
		CorrelationID: corrID,
	}
}

func DeploymentFailure(orgID, userID, projectID, deploymentID, reason, recipient, corrID string) model.Notification {
	return model.Notification{
		OrgID:         orgID,
		UserID:        userID,
		ProjectID:     projectID,
		Channel:       model.ChannelEmail,
		EventType:     "deployment.failure",
		Subject:       "Deployment failed",
		Body:          fmt.Sprintf("Deployment %s failed: %s", deploymentID, reason),
		Recipient:     recipient,
		CorrelationID: corrID,
	}
}

func QuotaWarning(orgID, recipient string, dim model.UsageDimension, pct float64, corrID string) model.Notification {
	return model.Notification{
		OrgID:         orgID,
		Channel:       model.ChannelEmail,
		EventType:     "quota.warning",
		Subject:       fmt.Sprintf("Usage warning: %s at %.0f%%", dim, pct),
		Body:          fmt.Sprintf("Your %s usage has reached %.1f%% of your plan limit.", dim, pct),
		Recipient:     recipient,
		CorrelationID: corrID,
	}
}

func SSLExpiring(orgID, hostname, recipient, corrID string, daysLeft int) model.Notification {
	return model.Notification{
		OrgID:         orgID,
		Channel:       model.ChannelEmail,
		EventType:     "ssl.expiring",
		Subject:       fmt.Sprintf("SSL certificate expiring soon: %s", hostname),
		Body:          fmt.Sprintf("The SSL certificate for %s will expire in %d days.", hostname, daysLeft),
		Recipient:     recipient,
		CorrelationID: corrID,
	}
}
