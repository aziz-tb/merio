package tagclassification

import (
	"context"
	"fmt"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	"github.com/aziz-tb/merio/internal/realtime"
)

func (a *Activities) publishTagsCompleted(ctx context.Context, convUUID, status, traceID, errorMessage string) error {
	if status == classificationStatusSkipped {
		status = aiopmodels.StatusSuccess
	}
	event, err := realtime.NewEvent(realtime.EventTagClassificationCompleted, realtime.WorkflowCompletedEvent{
		Type:             realtime.EventTagClassificationCompleted,
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

func (a *Activities) publishClassificationFailure(ctx context.Context, convUUID, errorMessage string) error {
	traceID := fmt.Sprintf("tag-classify:%s", convUUID)
	return a.publishTagsCompleted(ctx, convUUID, classificationStatusFailed, traceID, errorMessage)
}
