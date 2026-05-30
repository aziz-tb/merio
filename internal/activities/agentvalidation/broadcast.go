package agentvalidation

import (
	"context"

	aiopmodels "github.com/abhinavxd/libredesk/internal/ai/operations/models"
	"github.com/abhinavxd/libredesk/internal/realtime"
)

func (a *Activities) publishValidationCompleted(ctx context.Context, convUUID, status, traceID, errorMessage string) error {
	if status == statusSkipped {
		status = aiopmodels.StatusSuccess
	}
	event, err := realtime.NewEvent(realtime.EventAgentValidationCompleted, realtime.WorkflowCompletedEvent{
		Type:             realtime.EventAgentValidationCompleted,
		ConversationUUID: convUUID,
		Status:           status,
		TraceID:          traceID,
		Error:            errorMessage,
	})
	if err != nil {
		return err
	}
	return a.Realtime.Publish(ctx, event)
}
