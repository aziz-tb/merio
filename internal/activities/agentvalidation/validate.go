package agentvalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	aiopmodels "github.com/aziz-tb/merio/internal/ai/operations/models"
	"github.com/aziz-tb/merio/internal/ai/operations/payloads"
	contractpkg "github.com/aziz-tb/merio/internal/contract"
	customerpkg "github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/volatiletech/null/v9"
)

const (
	triggeredByManual = "manual"
	statusSuccess     = aiopmodels.StatusSuccess
	statusSkipped     = aiopmodels.StatusSkipped
)

type Input struct {
	ConversationUUID  string
	TriggeredBy       string
	TriggeredByUserID int64
}

type Output struct {
	OperationID      int64
	MatchingAgentIDs []int
	Status           string
}

func (a *Activities) ValidateAgentsActivity(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	if input.TriggeredBy == "" {
		input.TriggeredBy = triggeredByManual
	}
	traceID := fmt.Sprintf("agent-validate:%s", input.ConversationUUID)

	conv, err := a.ConversationMgr.GetConversation(0, input.ConversationUUID, "")
	if err != nil {
		return Output{}, fmt.Errorf("load conversation: %w", err)
	}

	enabled, err := a.AgentsMgr.ListEnabled(ctx)
	if err != nil {
		return Output{}, fmt.Errorf("load enabled agents: %w", err)
	}
	if len(enabled) == 0 {
		opID, err := a.persistSkipped(ctx, conv.ID, conv.UUID, input, traceID, "no enabled agents")
		if err != nil {
			return Output{}, err
		}
		return Output{OperationID: opID, Status: statusSkipped}, nil
	}

	ec := detection.EvaluationContext{
		Conversation: conv,
		Tags:         conv.TagSlugs(),
		TraceID:      traceID,
	}
	if conv.CustomerID.Valid && a.CustomerMgr != nil {
		customer, err := a.CustomerMgr.GetFreshByCustomerID(ctx, conv.CustomerID.String, customerpkg.FreshOptions{})
		if err != nil {
			a.Lo.Warn("agentvalidation: failed to hydrate customer for detection", "conversation_uuid", conv.UUID, "customer_id", conv.CustomerID.String, "error", err)
		} else {
			ec.Customer = customer
		}
	}
	if conv.ContractID.Valid && a.ContractMgr != nil {
		contract, err := a.ContractMgr.GetFreshByContractID(ctx, conv.ContractID.String, contractpkg.FreshOptions{})
		if err != nil {
			a.Lo.Warn("agentvalidation: failed to hydrate contract for detection", "conversation_uuid", conv.UUID, "contract_id", conv.ContractID.String, "error", err)
		} else {
			ec.Contract = contract
		}
	}

	matchingIDs := make([]int, 0)
	allConsidered := make([]int, 0, len(enabled))
	perAgent := map[string]payloads.AgentValidationDetail{}

	for _, agent := range enabled {
		allConsidered = append(allConsidered, agent.ID)

		det, err := decodeAgentDetection(agent.Detection)
		if err != nil {
			a.Lo.Warn("agent has invalid detection JSON, skipping", "agent_id", agent.ID, "error", err)
			perAgent[fmt.Sprint(agent.ID)] = payloads.AgentValidationDetail{
				Matched:   false,
				Reasoning: fmt.Sprintf("invalid detection: %v", err),
			}
			continue
		}

		result, err := a.DetectionMgr.Evaluate(ctx, det, ec)
		if err != nil {
			a.Lo.Warn("agent detection evaluation failed", "agent_id", agent.ID, "error", err)
			perAgent[fmt.Sprint(agent.ID)] = payloads.AgentValidationDetail{
				Matched:   false,
				Reasoning: fmt.Sprintf("evaluation error: %v", err),
			}
			continue
		}

		if result.Matched {
			matchingIDs = append(matchingIDs, agent.ID)
		}
		if result.Reasoning != "" || result.Score > 0 {
			perAgent[fmt.Sprint(agent.ID)] = payloads.AgentValidationDetail{
				Matched:    result.Matched,
				Confidence: result.Score,
				Reasoning:  result.Reasoning,
			}
		}
	}

	payload := payloads.AgentValidationPayload{
		MatchingAgentIDs:      matchingIDs,
		PerAgent:              perAgent,
		AllConsideredAgentIDs: allConsidered,
	}
	raw, err := payload.Marshal()
	if err != nil {
		return Output{}, fmt.Errorf("marshal payload: %w", err)
	}

	opID, err := a.insertOperation(ctx, operationRow{
		ConversationID: conv.ID,
		TraceID:        traceID,
		Input:          input,
		Status:         statusSuccess,
		Payload:        raw,
	})
	if err != nil {
		return Output{}, fmt.Errorf("insert ai_operation: %w", err)
	}

	if err := a.publishValidationCompleted(ctx, conv.UUID, statusSuccess, traceID, ""); err != nil {
		a.Lo.Warn("publish agent validation update failed", "conversation_uuid", conv.UUID, "error", err)
	}

	a.Lo.Info("agent validation completed",
		"conversation_uuid", conv.UUID,
		"considered", len(allConsidered),
		"matched", len(matchingIDs),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return Output{OperationID: opID, MatchingAgentIDs: matchingIDs, Status: statusSuccess}, nil
}

func decodeAgentDetection(raw json.RawMessage) (detection.Detection, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return detection.Detection{Mode: detection.ModeManualOnly}, nil
	}
	var det detection.Detection
	if err := json.Unmarshal(raw, &det); err != nil {
		return detection.Detection{}, err
	}
	if det.Mode == "" {
		det.Mode = detection.ModeManualOnly
	}
	return det, nil
}

func (a *Activities) persistSkipped(ctx context.Context, convID int, convUUID string, input Input, traceID, reason string) (int64, error) {
	payload := payloads.AgentValidationPayload{
		MatchingAgentIDs:      []int{},
		AllConsideredAgentIDs: []int{},
	}
	raw, err := payload.Marshal()
	if err != nil {
		return 0, fmt.Errorf("marshal skipped payload: %w", err)
	}

	opID, err := a.insertOperation(ctx, operationRow{
		ConversationID: convID,
		TraceID:        traceID,
		Input:          input,
		Status:         statusSkipped,
		ErrorMessage:   reason,
		Payload:        raw,
	})
	if err != nil {
		return 0, fmt.Errorf("insert skipped ai_operation: %w", err)
	}
	if err := a.publishValidationCompleted(ctx, convUUID, statusSkipped, traceID, ""); err != nil {
		a.Lo.Warn("publish skipped agent validation update failed", "conversation_uuid", convUUID, "error", err)
	}
	return opID, nil
}

type operationRow struct {
	ConversationID int
	TraceID        string
	Input          Input
	Status         string
	ErrorMessage   string
	Payload        json.RawMessage
}

func (a *Activities) insertOperation(ctx context.Context, row operationRow) (int64, error) {
	op := aiopmodels.Operation{
		OperationType:     aiopmodels.TypeAgentValidation,
		ConversationID:    null.Int64From(int64(row.ConversationID)),
		TriggeredBy:       row.Input.TriggeredBy,
		TriggeredByUserID: nullInt64Positive(row.Input.TriggeredByUserID),
		TraceID:           row.TraceID,
		Status:            row.Status,
		ErrorMessage:      nullStringNonEmpty(row.ErrorMessage),
		Payload:           row.Payload,
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
