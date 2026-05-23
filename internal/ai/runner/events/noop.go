package events

import (
	"context"

	"github.com/abhinavxd/libredesk/internal/ai/runner"
)

type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ runner.RunEvent) error {
	return nil
}
