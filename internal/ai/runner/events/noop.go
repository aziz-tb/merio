package events

import (
	"context"

	"github.com/aziz-tb/merio/internal/ai/runner"
)

type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ runner.RunEvent) error {
	return nil
}
