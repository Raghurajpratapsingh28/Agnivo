package notification_test

import (
	"context"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifBuilders(t *testing.T) {
	n := notification.DeploymentSuccess("org1", "user1", "proj1", "dep1", "test@example.com", "corr1")
	assert.Equal(t, "deployment.success", n.EventType)
	assert.Equal(t, model.ChannelEmail, n.Channel)
	assert.Contains(t, n.Body, "dep1")

	f := notification.DeploymentFailure("org1", "user1", "proj1", "dep1", "OOM", "test@example.com", "corr1")
	assert.Equal(t, "deployment.failure", f.EventType)
	assert.Contains(t, f.Body, "OOM")

	w := notification.QuotaWarning("org1", "test@example.com", model.DimBandwidthGB, 90.5, "corr1")
	assert.Equal(t, "quota.warning", w.EventType)
	assert.Contains(t, w.Subject, "90%")

	ssl := notification.SSLExpiring("org1", "example.com", "test@example.com", "corr1", 7)
	assert.Equal(t, "ssl.expiring", ssl.EventType)
	assert.Contains(t, ssl.Body, "7 days")
}

type mockSender struct {
	called bool
	err    error
}

func (m *mockSender) Send(_ context.Context, _ model.Notification) error {
	m.called = true
	return m.err
}

func TestDispatcher_NoRepo(t *testing.T) {
	// Verify dispatcher construction doesn't panic.
	d := notification.NewDispatcher(nil, notification.SMTPConfig{}, nil)
	require.NotNil(t, d)
}

func TestChannelConstants(t *testing.T) {
	channels := []model.NotificationChannel{
		model.ChannelEmail, model.ChannelSlack, model.ChannelDiscord,
		model.ChannelWebhook, model.ChannelInApp,
	}
	for _, ch := range channels {
		assert.NotEmpty(t, string(ch))
	}
}

func TestNotificationStatuses(t *testing.T) {
	assert.Equal(t, model.NotificationStatus("pending"), model.NotifPending)
	assert.Equal(t, model.NotificationStatus("delivered"), model.NotifDelivered)
	assert.Equal(t, model.NotificationStatus("failed"), model.NotifFailed)
}
