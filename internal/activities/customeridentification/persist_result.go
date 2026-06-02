package customeridentification

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	aiopmodels "github.com/abhinavxd/libredesk/internal/ai/operations/models"
	"github.com/abhinavxd/libredesk/internal/ai/operations/payloads"
	"github.com/abhinavxd/libredesk/internal/realtime"
	verificationmodels "github.com/abhinavxd/libredesk/internal/verification/models"
	cimodels "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
)

type previousLinks struct {
	UUID             string         `db:"uuid"`
	ContactID        int            `db:"contact_id"`
	CustomerID       sql.NullString `db:"customer_id"`
	ContractID       sql.NullString `db:"contract_id"`
	LinkedCustomerID sql.NullString `db:"linked_customer_id"`
}

func (a *Activities) PersistResultActivity(ctx context.Context, in cimodels.PersistResultInput) (cimodels.PersistResultOutput, error) {
	tx, err := a.DB.BeginTxx(ctx, nil)
	if err != nil {
		return cimodels.PersistResultOutput{}, err
	}
	defer tx.Rollback()

	var prev previousLinks
	if err := tx.GetContext(ctx, &prev, a.q.GetConversationIdentificationLinks, in.ConversationID); err != nil {
		return cimodels.PersistResultOutput{}, err
	}

	if in.Status == cimodels.StatusSuccess || in.Status == cimodels.StatusNoMatch {
		var verificationMessage any
		if in.Verification.Message != "" {
			verificationMessage = in.Verification.Message
		}
		if _, err := tx.ExecContext(ctx, a.q.UpdateConversationIdentification,
			in.ConversationID,
			nullableStringPtr(in.CustomerID),
			nullableStringPtr(in.ContractID),
			in.Verification.Level,
			verificationMessage,
		); err != nil {
			return cimodels.PersistResultOutput{}, err
		}

		if in.CustomerID != nil && in.Verification.Level >= int(verificationmodels.VerificationLevelVerified) {
			if _, err := tx.ExecContext(ctx, a.q.UpdateContactLinkedCustomer, prev.ContactID, *in.CustomerID); err != nil {
				return cimodels.PersistResultOutput{}, err
			}
		}
	}

	payload, err := buildPayload(in, prev)
	if err != nil {
		return cimodels.PersistResultOutput{}, err
	}
	status := aiOperationStatus(in.Status)
	if in.AIOperationID <= 0 {
		return cimodels.PersistResultOutput{}, fmt.Errorf("customeridentification: ai_operation_id required")
	}
	result, err := tx.ExecContext(ctx, a.q.UpdateAIOperation,
		in.AIOperationID,
		status,
		nullableString(in.ErrorMessage),
		[]byte(payload),
	)
	if err != nil {
		return cimodels.PersistResultOutput{}, err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return cimodels.PersistResultOutput{}, fmt.Errorf("customeridentification: ai_operation %d not found", in.AIOperationID)
	}
	if err := tx.Commit(); err != nil {
		return cimodels.PersistResultOutput{}, err
	}
	previousLinkedCustomerID := nullStringPtr(prev.LinkedCustomerID)
	if err := a.publishCustomerIdentificationCompleted(ctx, prev.UUID, in, previousLinkedCustomerID); err != nil {
		a.Lo.Warn("customer identification realtime publish failed",
			"conversation_uuid", prev.UUID,
			"trace_id", in.TraceID,
			"error", err)
	}
	return cimodels.PersistResultOutput{
		AIOperationID:            in.AIOperationID,
		PreviousLinkedCustomerID: previousLinkedCustomerID,
	}, nil
}

func (a *Activities) publishCustomerIdentificationCompleted(ctx context.Context, convUUID string, in cimodels.PersistResultInput, previousLinkedCustomerID *string) error {
	status := aiopmodels.StatusSuccess
	if in.Status == cimodels.StatusFailed {
		status = aiopmodels.StatusFailed
	}
	event, err := realtime.NewEvent(realtime.EventCustomerIdentificationCompleted, realtime.WorkflowCompletedEvent{
		Type:                     realtime.EventCustomerIdentificationCompleted,
		ConversationUUID:         convUUID,
		Status:                   status,
		TraceID:                  in.TraceID,
		Error:                    operatorError(in.ErrorMessage),
		PreviousLinkedCustomerID: previousLinkedCustomerID,
	})
	if err != nil {
		return err
	}
	return a.Realtime.Publish(ctx, event)
}

func operatorError(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	const max = 500
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

func buildPayload(in cimodels.PersistResultInput, prev previousLinks) (json.RawMessage, error) {
	var msg *string
	if in.Verification.Message != "" {
		msg = &in.Verification.Message
	}
	matched := criterionNames(in.Verification.Matched)
	failed := criterionNames(in.Verification.Failed)
	payload := payloads.CustomerIdentificationPayload{
		Method:                 in.Method,
		Channel:                in.Channel,
		SkipReason:             in.SkipReason,
		PatternDetection:       in.PatternDetection,
		LLMExtraction:          in.LLMExtraction,
		SearchCustomerByFields: in.SearchByFields,
		Load:                   in.Load,
		Verification: payloads.CustomerVerificationResult{
			Level:        in.Verification.Level,
			Source:       in.Verification.Source,
			Message:      msg,
			RulesMatched: matched,
			RulesFailed:  failed,
		},
		Result: payloads.CustomerIdentificationLink{
			CustomerID:         in.CustomerID,
			ContractID:         in.ContractID,
			PreviousCustomerID: nullStringPtr(prev.CustomerID),
			PreviousContractID: nullStringPtr(prev.ContractID),
		},
	}
	return payload.Marshal()
}

func criterionNames(in []verificationmodels.CriterionResult) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		if r.Name != "" {
			out = append(out, r.Name)
			continue
		}
		if r.Label != "" {
			out = append(out, r.Label)
		}
	}
	return out
}

func aiOperationStatus(status string) string {
	switch status {
	case cimodels.StatusFailed:
		return aiopmodels.StatusFailed
	case cimodels.StatusSkipped:
		return aiopmodels.StatusSkipped
	default:
		return aiopmodels.StatusSuccess
	}
}

func triggeredBy(userID *int64) string {
	if userID == nil {
		return aiopmodels.TriggeredByAuto
	}
	return aiopmodels.TriggeredByManual
}

func nullableStringPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullStringPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}
