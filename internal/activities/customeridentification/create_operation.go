package customeridentification

import (
	"context"
	"encoding/json"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	cimodels "github.com/aziz-tb/merio/internal/workflows/customeridentification"
)

func (a *Activities) CreateOperationActivity(ctx context.Context, in cimodels.CreateOperationInput) (cimodels.CreateOperationOutput, error) {
	payload, err := json.Marshal(map[string]any{
		"channel": in.Channel,
		"method":  cimodels.MethodNone,
	})
	if err != nil {
		return cimodels.CreateOperationOutput{}, err
	}

	var opID int64
	if err := a.DB.QueryRowxContext(ctx, a.q.InsertAIOperation,
		aiopmodels.TypeCustomerIdentification,
		in.ConversationID,
		triggeredBy(in.TriggeredByUserID),
		nullableInt64(in.TriggeredByUserID),
		in.TraceID,
		aiopmodels.StatusRunning,
		nil,
		payload,
	).Scan(&opID); err != nil {
		return cimodels.CreateOperationOutput{}, err
	}
	return cimodels.CreateOperationOutput{AIOperationID: opID}, nil
}
