package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"
)

// RedisPublisher publishes realtime events to a Redis pub/sub channel.
type RedisPublisher struct {
	client  *redis.Client
	channel string
}

// NewRedisPublisher returns a RedisPublisher on the default channel; it errors if client is nil.
func NewRedisPublisher(client *redis.Client) (*RedisPublisher, error) {
	if client == nil {
		return nil, errors.New("realtime: redis client is required")
	}
	return &RedisPublisher{client: client, channel: DefaultChannel}, nil
}

// Publish marshals the event and publishes it to the channel; it errors if the event has no Type.
func (p *RedisPublisher) Publish(ctx context.Context, event Event) error {
	if event.Type == "" {
		return errors.New("realtime: event type is required")
	}
	if len(event.Data) == 0 {
		event.Data = json.RawMessage("null")
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, p.channel, b).Err()
}

// RedisSubscriber consumes realtime events from the Redis channel and dispatches them to a Handler.
type RedisSubscriber struct {
	client  *redis.Client
	channel string
	lo      *logf.Logger
}

// NewRedisSubscriber returns a RedisSubscriber; it errors if client or logger is nil.
func NewRedisSubscriber(client *redis.Client, lo *logf.Logger) (*RedisSubscriber, error) {
	if client == nil {
		return nil, errors.New("realtime: redis client is required")
	}
	if lo == nil {
		return nil, errors.New("realtime: logger is required")
	}
	return &RedisSubscriber{client: client, channel: DefaultChannel, lo: lo}, nil
}

// Run subscribes and dispatches each event to handler until ctx is cancelled. Malformed or
// type-less events are logged and skipped, and handler errors are logged rather than fatal.
func (s *RedisSubscriber) Run(ctx context.Context, handler Handler) error {
	if handler == nil {
		return errors.New("realtime: handler is required")
	}

	pubsub := s.client.Subscribe(ctx, s.channel)
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				s.lo.Warn("realtime: ignoring malformed event", "error", err)
				continue
			}
			if event.Type == "" {
				s.lo.Warn("realtime: ignoring event without type")
				continue
			}
			if err := handler(ctx, event); err != nil {
				s.lo.Warn("realtime: handler error", "type", event.Type, "error", err)
			}
		}
	}
}
