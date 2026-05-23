package agentvalidation

import (
	"context"

	"github.com/abhinavxd/libredesk/internal/realtime"
)

type validationCompletedPayload struct {
	ConversationUUID string `json:"conversation_uuid"`
	OperationID      int64  `json:"operation_id"`
	MatchingAgentIDs []int  `json:"matching_agent_ids"`
}

func (a *Activities) publishValidationCompleted(ctx context.Context, convUUID string, operationID int64, matchingIDs []int) error {
	event, err := realtime.NewEvent(realtime.EventAgentValidationCompleted, validationCompletedPayload{
		ConversationUUID: convUUID,
		OperationID:      operationID,
		MatchingAgentIDs: matchingIDs,
	})
	if err != nil {
		return err
	}
	return a.Realtime.Publish(ctx, event)
}
