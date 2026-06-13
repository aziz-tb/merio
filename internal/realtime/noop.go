package realtime

import "context"

// NoopPublisher is a Publisher that discards events, used when realtime is disabled.
type NoopPublisher struct{}

// Publish discards the event and returns nil.
func (NoopPublisher) Publish(context.Context, Event) error {
	return nil
}
