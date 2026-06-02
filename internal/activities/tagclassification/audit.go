package tagclassification

import (
	"context"
	"fmt"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	"github.com/aziz-tb/merio/internal/ai/operations/payloads"
	"github.com/volatiletech/null/v9"
)

// classificationRow is the activity-internal shape passed to
// insertClassification. It is flattened into an ai_operations row of type
// tag_classification plus a TagClassificationPayload. Token counts and
// latency are NOT carried here — they land in ai_usage via recordAIUsage,
// joined to this operation by trace_id.
type classificationRow struct {
	ConversationID    int
	TraceID           string
	TriggeredBy       string
	TriggeredByUserID int64
	LLMReasoning      string
	LLMScores         map[string]float64
	LLMModel          string
	CandidateTagIDs   []int
	AppliedTagIDs     []int
	RemovedTagIDs     []int
	Status            string
	ErrorMessage      string
}

// insertClassification writes one ai_operations row of type
// tag_classification and returns its id. The id is stored on
// conversation_tags.ai_operation_id for AI-applied tags and broadcast to
// the UI in the realtime tags-update event.
func (a *Activities) insertClassification(ctx context.Context, row classificationRow) (int64, error) {
	payload := payloads.TagClassificationPayload{
		CandidateTagIDs: row.CandidateTagIDs,
		AppliedTagIDs:   row.AppliedTagIDs,
		RemovedTagIDs:   row.RemovedTagIDs,
		LLMScores:       row.LLMScores,
		Reasoning:       row.LLMReasoning,
		LLMModel:        row.LLMModel,
	}
	raw, err := payload.Marshal()
	if err != nil {
		return 0, fmt.Errorf("marshal tag classification payload: %w", err)
	}

	op := aiopmodels.Operation{
		OperationType:     aiopmodels.TypeTagClassification,
		ConversationID:    null.Int64From(int64(row.ConversationID)),
		TriggeredBy:       row.TriggeredBy,
		TriggeredByUserID: nullInt64Positive(row.TriggeredByUserID),
		TraceID:           row.TraceID,
		Status:            row.Status,
		ErrorMessage:      nullStringNonEmpty(row.ErrorMessage),
		Payload:           raw,
	}
	return a.AIOperationsMgr.Insert(ctx, op)
}

func nullInt64Positive(id int64) null.Int64 {
	if id > 0 {
		return null.Int64From(id)
	}
	return null.Int64{}
}

func nullStringNonEmpty(s string) null.String {
	if s == "" {
		return null.String{}
	}
	return null.StringFrom(s)
}
