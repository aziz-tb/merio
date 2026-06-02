package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/ai/runner"
)

const (
	streamFieldJSON = "json"
	streamMaxLen    = 1000
	defaultEventTTL = time.Hour
)

type RedisStreamPublisher struct {
	client *redis.Client
	lo     *logf.Logger
	ttl    time.Duration
}

func NewRedisStreamPublisher(client *redis.Client, lo *logf.Logger, ttl time.Duration) (*RedisStreamPublisher, error) {
	if client == nil {
		return nil, errors.New("runner events: redis client is required")
	}
	if lo == nil {
		return nil, errors.New("runner events: logger is required")
	}
	if ttl <= 0 {
		ttl = defaultEventTTL
	}
	return &RedisStreamPublisher{
		client: client,
		lo:     lo,
		ttl:    ttl,
	}, nil
}

func (p *RedisStreamPublisher) Publish(ctx context.Context, traceID string, event runner.RunEvent) error {
	if traceID == "" {
		return errors.New("runner events: traceID is required")
	}
	if event.EmittedAt.IsZero() {
		event.EmittedAt = time.Now().UTC()
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	key := StreamKey(traceID)
	pipe := p.client.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: map[string]any{
			streamFieldJSON: string(body),
		},
	})
	pipe.Expire(ctx, key, p.ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func StreamKey(traceID string) string {
	return "ai:run_events:" + traceID
}
