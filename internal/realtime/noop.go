package realtime

import "context"

type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Event) error {
	return nil
}
