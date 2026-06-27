package redis

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	goredis "github.com/redis/go-redis/v9"
)

// StreamMessage is a single entry read from a stream.
type StreamMessage struct {
	ID     string
	Values map[string]any
}

// StreamAdd appends an entry to a stream, optionally trimming to an approximate
// maxLen (0 disables trimming). It returns the generated entry ID.
func (c *Client) StreamAdd(ctx context.Context, stream string, values map[string]any, maxLen int64) (string, error) {
	args := &goredis.XAddArgs{Stream: stream, Values: values}
	if maxLen > 0 {
		args.MaxLen = maxLen
		args.Approx = true // '~' trimming is far cheaper at high throughput
	}
	id, err := c.Client.XAdd(ctx, args).Result()
	if err != nil {
		return "", errors.Wrap(err, errors.CodeUnavailable, "redis: stream add")
	}
	return id, nil
}

// EnsureGroup creates a consumer group at the stream's end, creating the stream
// if needed. It is idempotent: an existing group is not treated as an error.
func (c *Client) EnsureGroup(ctx context.Context, stream, group string) error {
	err := c.Client.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: ensure group")
	}
	return nil
}

// StreamRead reads up to count new messages for a consumer in a group, blocking
// up to block for new entries. An empty result with no error means the block
// elapsed with nothing new.
func (c *Client) StreamRead(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]StreamMessage, error) {
	res, err := c.Client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, errors.Wrap(err, errors.CodeUnavailable, "redis: stream read")
	}
	var out []StreamMessage
	for _, s := range res {
		for _, m := range s.Messages {
			out = append(out, StreamMessage{ID: m.ID, Values: m.Values})
		}
	}
	return out, nil
}

// StreamAck acknowledges processed message IDs for a consumer group.
func (c *Client) StreamAck(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := c.Client.XAck(ctx, stream, group, ids...).Err(); err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: stream ack")
	}
	return nil
}
