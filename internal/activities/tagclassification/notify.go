package tagclassification

import (
	"context"
	"fmt"
)

type FailureInput struct {
	ConversationUUID string
	ErrorMessage     string
}

func (a *Activities) NotifyClassificationFailureActivity(ctx context.Context, input FailureInput) error {
	if input.ConversationUUID == "" {
		return fmt.Errorf("conversation uuid required")
	}
	if err := a.publishClassificationFailure(ctx, input.ConversationUUID, input.ErrorMessage); err != nil {
		return fmt.Errorf("publish failure update: %w", err)
	}
	return nil
}
