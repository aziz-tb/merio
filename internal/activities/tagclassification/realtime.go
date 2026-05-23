package tagclassification

import (
	"context"

	"github.com/abhinavxd/libredesk/internal/realtime"
)

type classificationUpdate struct {
	Status        string `json:"status"`
	AIOperationID int64  `json:"ai_operation_id,omitempty"`
	Error         string `json:"error,omitempty"`
}

type conversationUpdate struct {
	UUID              string               `json:"uuid"`
	Tags              any                  `json:"tags,omitempty"`
	TagIDs            any                  `json:"tag_ids,omitempty"`
	TagClassification classificationUpdate `json:"tag_classification"`
}

func (a *Activities) publishTagsUpdate(ctx context.Context, convUUID, status string, aiOperationID int64) error {
	conv, err := a.ConversationMgr.GetConversation(0, convUUID, "")
	if err != nil {
		return err
	}
	return a.publishClassificationUpdate(ctx, conversationUpdate{
		UUID:   convUUID,
		Tags:   conv.Tags,
		TagIDs: conv.TagIDs,
		TagClassification: classificationUpdate{
			Status:        status,
			AIOperationID: aiOperationID,
		},
	})
}

func (a *Activities) publishClassificationFailure(ctx context.Context, convUUID string) error {
	return a.publishClassificationUpdate(ctx, conversationUpdate{
		UUID: convUUID,
		TagClassification: classificationUpdate{
			Status: classificationStatusFailed,
			Error:  "Classification failed.",
		},
	})
}

func (a *Activities) publishClassificationUpdate(ctx context.Context, payload conversationUpdate) error {
	event, err := realtime.NewEvent(realtime.EventConversationUpdate, payload)
	if err != nil {
		return err
	}
	return a.Realtime.Publish(ctx, event)
}
