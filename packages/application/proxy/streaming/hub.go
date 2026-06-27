// Package streaming implements the edge live-streaming platform:
// a Redis Pub/Sub-backed fan-out hub that distributes proxy events, deployment
// logs, build progress, container events, and runtime metrics to SSE clients.
// Multiple proxy-manager instances share the same Redis channels so any
// instance can serve any subscriber.
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"go.uber.org/zap"
)

// Channel name patterns for Redis pub/sub.
const (
	channelProxy      = "proxy:events"
	channelDeployment = "proxy:deployment:"
	channelProject    = "proxy:project:"
	channelOrg        = "proxy:org:"
	channelBroadcast  = "proxy:broadcast"
)

// Subscriber is a fan-out recipient for streaming messages.
type Subscriber struct {
	id      string
	channel string
	ch      chan model.StreamMessage
}

// Hub manages Redis pub/sub subscriptions and local fan-out to SSE clients.
type Hub struct {
	redis *redis.Client
	log   *zap.Logger

	mu   sync.RWMutex
	subs map[string][]*Subscriber // key = channel name

	// Atomic counters for metrics.
	activeConns    int64
	totalPublished int64
}

// NewHub creates a new streaming Hub.
func NewHub(redisClient *redis.Client, log *zap.Logger) *Hub {
	return &Hub{
		redis: redisClient,
		log:   log,
		subs:  make(map[string][]*Subscriber),
	}
}

// Run starts the Redis subscription loops. It blocks until ctx is canceled.
// Run must be called in a separate goroutine.
func (h *Hub) Run(ctx context.Context) error {
	channels := []string{
		channelProxy,
		channelBroadcast,
	}
	return h.redis.Subscribe(ctx, func(ctx context.Context, channel string, payload []byte) error {
		return h.dispatch(channel, payload)
	}, channels...)
}

// Publish sends a message to a Redis channel for cross-instance fan-out.
func (h *Hub) Publish(ctx context.Context, msg model.StreamMessage) error {
	if msg.ID == "" {
		msg.ID = idx.NewUUID()
	}
	msg.Timestamp = time.Now().UTC()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ch := channelForMessage(msg)
	_, err = h.redis.Publish(ctx, ch, data)
	if err != nil {
		return err
	}
	atomic.AddInt64(&h.totalPublished, 1)
	return nil
}

// Subscribe registers a local SSE subscriber for a channel. The returned
// Subscriber carries a buffered channel; the caller reads from it until the
// provided context is canceled, then calls Unsubscribe.
func (h *Hub) Subscribe(ctx context.Context, channel string, bufSize int) *Subscriber {
	if bufSize <= 0 {
		bufSize = 64
	}
	sub := &Subscriber{
		id:      idx.NewUUID(),
		channel: channel,
		ch:      make(chan model.StreamMessage, bufSize),
	}
	h.mu.Lock()
	h.subs[channel] = append(h.subs[channel], sub)
	h.mu.Unlock()
	atomic.AddInt64(&h.activeConns, 1)

	// Auto-unsubscribe on context cancellation.
	go func() {
		<-ctx.Done()
		h.Unsubscribe(sub)
	}()

	return sub
}

// Unsubscribe removes and closes a subscriber.
func (h *Hub) Unsubscribe(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.subs[sub.channel]
	for i, s := range list {
		if s.id == sub.id {
			h.subs[sub.channel] = append(list[:i], list[i+1:]...)
			close(s.ch)
			atomic.AddInt64(&h.activeConns, -1)
			return
		}
	}
}

// Messages returns the receive channel for a subscriber.
func (s *Subscriber) Messages() <-chan model.StreamMessage { return s.ch }

// ChannelForDeployment returns the channel name for a deployment stream.
func ChannelForDeployment(deploymentID string) string {
	return channelDeployment + deploymentID
}

// ChannelForProject returns the channel name for a project stream.
func ChannelForProject(projectID string) string {
	return channelProject + projectID
}

// ChannelForOrg returns the channel name for an org-level stream.
func ChannelForOrg(orgID string) string {
	return channelOrg + orgID
}

// Stats returns current connection statistics.
func (h *Hub) Stats() model.ConnectionStats {
	h.mu.RLock()
	total := int64(0)
	for _, list := range h.subs {
		total += int64(len(list))
	}
	h.mu.RUnlock()
	return model.ConnectionStats{
		SSEConnections:      atomic.LoadInt64(&h.activeConns),
		ActiveSubscriptions: total,
	}
}

// dispatch delivers a raw Redis message to all local subscribers for that channel.
func (h *Hub) dispatch(channel string, payload []byte) error {
	var msg model.StreamMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.log.Warn("streaming: malformed message",
			zap.String("channel", channel),
			zap.Error(err))
		return nil
	}

	h.mu.RLock()
	subs := h.subs[channel]
	// Also fan out to wildcard/broadcast subscribers.
	broadcastSubs := h.subs[channelBroadcast]
	all := make([]*Subscriber, 0, len(subs)+len(broadcastSubs))
	all = append(all, subs...)
	all = append(all, broadcastSubs...)
	h.mu.RUnlock()

	for _, sub := range all {
		select {
		case sub.ch <- msg:
		default:
			// Subscriber is slow; drop rather than block the dispatch loop.
			h.log.Debug("streaming: subscriber buffer full, dropping message",
				zap.String("sub_id", sub.id))
		}
	}
	return nil
}

// Heartbeat sends a keep-alive ping to all active subscribers.
// Should be called periodically (e.g. every 30 seconds) to detect dead clients.
func (h *Hub) Heartbeat(ctx context.Context) {
	ping := model.StreamMessage{
		ID:        idx.NewUUID(),
		Channel:   channelBroadcast,
		EventType: "ping",
		Timestamp: time.Now().UTC(),
	}
	h.mu.RLock()
	for channel, subs := range h.subs {
		for _, sub := range subs {
			ping.Channel = channel
			select {
			case sub.ch <- ping:
			default:
			}
		}
	}
	h.mu.RUnlock()
}

func channelForMessage(msg model.StreamMessage) string {
	switch {
	case msg.DeploymentID != "":
		return fmt.Sprintf("%s%s", channelDeployment, msg.DeploymentID)
	case msg.ProjectID != "":
		return fmt.Sprintf("%s%s", channelProject, msg.ProjectID)
	case msg.OrgID != "":
		return fmt.Sprintf("%s%s", channelOrg, msg.OrgID)
	default:
		return channelProxy
	}
}
