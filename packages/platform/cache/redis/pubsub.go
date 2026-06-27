package redis

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	goredis "github.com/redis/go-redis/v9"
)

// Publish sends payload to channel and returns the number of subscribers that
// received it.
func (c *Client) Publish(ctx context.Context, channel string, payload []byte) (int64, error) {
	n, err := c.Client.Publish(ctx, channel, payload).Result()
	if err != nil {
		return 0, errors.Wrap(err, errors.CodeUnavailable, "redis: publish")
	}
	return n, nil
}

// MessageHandler processes a single pub/sub message. Returning an error logs it
// but does not stop the subscription.
type MessageHandler func(ctx context.Context, channel string, payload []byte) error

// Subscribe subscribes to channels and invokes handler for each message until
// ctx is canceled. It owns the underlying subscription and closes it on return,
// making it safe to run directly as a lifecycle runner.
func (c *Client) Subscribe(ctx context.Context, handler MessageHandler, channels ...string) error {
	sub := c.Client.Subscribe(ctx, channels...)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return errors.New(errors.CodeUnavailable, "redis: subscription channel closed")
			}
			if err := handler(ctx, msg.Channel, []byte(msg.Payload)); err != nil {
				c.log.Warn("pubsub handler error: " + err.Error())
			}
		}
	}
}

// Raw exposes the underlying go-redis subscription for advanced patterns
// (pattern subscriptions, manual ack flows) not covered by Subscribe.
func (c *Client) Raw() *goredis.Client { return c.Client }
