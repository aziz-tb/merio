package tagclassification

import (
	"context"
	"fmt"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/usage"
	contractpkg "github.com/aziz-tb/merio/internal/contract"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	customerpkg "github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
	tagmodels "github.com/aziz-tb/merio/internal/tag/models"
	"github.com/lib/pq"
)

const aiConfidenceThreshold = 0.5

const (
	triggeredByAuto   = "auto"
	triggeredByManual = "manual"

	classificationStatusSuccess = "success"
	classificationStatusFailed  = "failed"
	classificationStatusSkipped = "skipped"
)

type Input struct {
	ConversationUUID  string
	TriggeredBy       string
	TriggeredByUserID int64
}

type Output struct {
	AIOperationID int64
	AppliedTagIDs []int
	RemovedTagIDs []int
	Status        string
}

func (a *Activities) ClassifyTagsActivity(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	if input.TriggeredBy == "" {
		input.TriggeredBy = triggeredByAuto
	}
	traceID := fmt.Sprintf("tag-classify:%s", input.ConversationUUID)

	conv, err := a.ConversationMgr.GetConversation(0, input.ConversationUUID, "")
	if err != nil {
		return Output{}, fmt.Errorf("load conversation: %w", err)
	}

	tags, err := a.TagMgr.GetEnabledForClassification()
	if err != nil {
		return Output{}, fmt.Errorf("load tags: %w", err)
	}
	if len(tags) == 0 {
		id, err := a.insertClassification(ctx, classificationRow{
			ConversationID:    conv.ID,
			TraceID:           traceID,
			TriggeredBy:       input.TriggeredBy,
			TriggeredByUserID: input.TriggeredByUserID,
			Status:            classificationStatusSkipped,
			LLMReasoning:      "no eligible tags",
		})
		if err != nil {
			return Output{}, fmt.Errorf("persist skipped classification: %w", err)
		}
		if err := a.publishTagsCompleted(ctx, conv.UUID, classificationStatusSkipped, traceID, ""); err != nil {
			a.Lo.Warn("publish skipped tag classification completed failed", "conversation_uuid", conv.UUID, "error", err)
		}
		return Output{AIOperationID: id, Status: classificationStatusSkipped}, nil
	}

	ec := a.detectionContext(ctx, conv, traceID)
	matchedTagIDs, aiCandidates, candidateIDs := a.prefilterTags(tags, ec)

	var aiResult Result
	var aiLatencyMs int
	if len(aiCandidates) > 0 {
		aiStart := time.Now()
		aiResult, err = a.runBatchedAI(ctx, conv, aiCandidates, traceID)
		aiLatencyMs = int(time.Since(aiStart).Milliseconds())
		if err != nil {
			id, insertErr := a.insertClassification(ctx, classificationRow{
				ConversationID:    conv.ID,
				TraceID:           traceID,
				TriggeredBy:       input.TriggeredBy,
				TriggeredByUserID: input.TriggeredByUserID,
				CandidateTagIDs:   candidateIDs,
				Status:            classificationStatusFailed,
				ErrorMessage:      err.Error(),
			})
			if insertErr != nil {
				return Output{}, fmt.Errorf("ai call failed (%v); persist failed classification: %w", err, insertErr)
			}
			return Output{AIOperationID: id, Status: classificationStatusFailed}, fmt.Errorf("ai call: %w", err)
		}

		for _, t := range aiCandidates {
			if aiResult.Scores[t.Name] >= aiConfidenceThreshold {
				matchedTagIDs = append(matchedTagIDs, t.ID)
			}
		}
	}

	appliedByMap, err := a.fetchAppliedByMap(ctx, conv.ID)
	if err != nil {
		return Output{}, fmt.Errorf("fetch applied_by map: %w", err)
	}

	toApply, toRemove := computeApplyRemove(matchedTagIDs, appliedByMap)
	aiOperationID, err := a.insertClassification(ctx, classificationRow{
		ConversationID:    conv.ID,
		TraceID:           traceID,
		TriggeredBy:       input.TriggeredBy,
		TriggeredByUserID: input.TriggeredByUserID,
		LLMReasoning:      aiResult.Reasoning,
		LLMScores:         aiResult.Scores,
		LLMModel:          aiResult.Model,
		CandidateTagIDs:   candidateIDs,
		AppliedTagIDs:     toApply,
		RemovedTagIDs:     toRemove,
		Status:            classificationStatusSuccess,
	})
	if err != nil {
		return Output{}, fmt.Errorf("insert classification: %w", err)
	}

	if err := a.applyTags(ctx, conv.ID, toApply, aiOperationID); err != nil {
		return Output{}, fmt.Errorf("apply tags: %w", err)
	}
	if err := a.removeTags(ctx, conv.ID, toRemove); err != nil {
		return Output{}, fmt.Errorf("remove tags: %w", err)
	}

	a.applyTagSideEffects(conv, toApply, tags)
	if err := a.publishTagsCompleted(ctx, conv.UUID, classificationStatusSuccess, traceID, ""); err != nil {
		a.Lo.Warn("publish tag classification completed failed", "conversation_uuid", conv.UUID, "error", err)
	}
	if len(aiCandidates) > 0 && aiResult.Model != "" {
		a.recordAIUsage(ctx, conv.ID, traceID, aiResult, aiLatencyMs)
	}

	a.Lo.Info("tag classification completed",
		"conversation_uuid", conv.UUID,
		"candidates", len(candidateIDs),
		"applied", len(toApply),
		"removed", len(toRemove),
		"ai_candidates", len(aiCandidates),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return Output{AIOperationID: aiOperationID, AppliedTagIDs: toApply, RemovedTagIDs: toRemove, Status: classificationStatusSuccess}, nil
}

func (a *Activities) detectionContext(ctx context.Context, conv cmodels.Conversation, traceID string) detection.EvaluationContext {
	ec := detection.EvaluationContext{
		Conversation: conv,
		Tags:         conv.TagSlugs(),
		TraceID:      traceID,
	}
	if conv.CustomerID.Valid && a.CustomerMgr != nil {
		customer, err := a.CustomerMgr.GetFreshByCustomerID(ctx, conv.CustomerID.String, customerpkg.FreshOptions{})
		if err != nil {
			a.Lo.Warn("tagclassification: failed to hydrate customer for detection", "conversation_uuid", conv.UUID, "customer_id", conv.CustomerID.String, "error", err)
		} else {
			ec.Customer = customer
		}
	}
	if conv.ContractID.Valid && a.ContractMgr != nil {
		contract, err := a.ContractMgr.GetFreshByContractID(ctx, conv.ContractID.String, contractpkg.FreshOptions{})
		if err != nil {
			a.Lo.Warn("tagclassification: failed to hydrate contract for detection", "conversation_uuid", conv.UUID, "contract_id", conv.ContractID.String, "error", err)
		} else {
			ec.Contract = contract
		}
	}
	return ec
}

func (a *Activities) prefilterTags(tags []tagmodels.Tag, ec detection.EvaluationContext) ([]int, []tagmodels.Tag, []int) {
	var matched []int
	var aiCandidates []tagmodels.Tag
	var candidateIDs []int
	for _, t := range tags {
		candidateIDs = append(candidateIDs, t.ID)
		switch t.Detection.Mode {
		case detection.ModeRulesOnly:
			if detection.EvaluateRules(t.Detection.Rules, ec, a.Lo) {
				matched = append(matched, t.ID)
			}
		case detection.ModeAIOnly:
			aiCandidates = append(aiCandidates, t)
		case detection.ModeRulesAndAI:
			if detection.EvaluateRules(t.Detection.Rules, ec, a.Lo) {
				aiCandidates = append(aiCandidates, t)
			}
		case detection.ModeRulesOrAI:
			if detection.EvaluateRules(t.Detection.Rules, ec, a.Lo) {
				matched = append(matched, t.ID)
			} else {
				aiCandidates = append(aiCandidates, t)
			}
		}
	}
	return matched, aiCandidates, candidateIDs
}

func (a *Activities) fetchAppliedByMap(ctx context.Context, convID int) (map[int]string, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT tag_id, applied_by FROM conversation_tags WHERE conversation_id = $1`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var by string
		if err := rows.Scan(&id, &by); err != nil {
			return nil, err
		}
		out[id] = by
	}
	return out, rows.Err()
}

func computeApplyRemove(matchedTagIDs []int, appliedByMap map[int]string) ([]int, []int) {
	matchedSet := intSet(matchedTagIDs)
	var toApply, toRemove []int
	for tagID, appliedBy := range appliedByMap {
		if !matchedSet[tagID] && appliedBy == tagmodels.AppliedByAI {
			toRemove = append(toRemove, tagID)
		}
	}
	for _, tagID := range matchedTagIDs {
		if _, exists := appliedByMap[tagID]; !exists {
			toApply = append(toApply, tagID)
		}
	}
	return toApply, toRemove
}

func (a *Activities) applyTags(ctx context.Context, convID int, tagIDs []int, aiOperationID int64) error {
	for _, tagID := range tagIDs {
		if _, err := a.DB.ExecContext(ctx, `
			INSERT INTO conversation_tags (conversation_id, tag_id, applied_by, ai_operation_id)
			VALUES ($1, $2, 'ai', $3)
			ON CONFLICT (conversation_id, tag_id) DO NOTHING`,
			convID, tagID, aiOperationID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (a *Activities) removeTags(ctx context.Context, convID int, tagIDs []int) error {
	if len(tagIDs) == 0 {
		return nil
	}
	_, err := a.DB.ExecContext(ctx,
		`DELETE FROM conversation_tags WHERE conversation_id = $1 AND tag_id = ANY($2)`,
		convID, pq.Array(tagIDs))
	return err
}

func (a *Activities) applyTagSideEffects(conv cmodels.Conversation, appliedTagIDs []int, allTags []tagmodels.Tag) {
	byID := map[int]tagmodels.Tag{}
	for _, t := range allTags {
		byID[t.ID] = t
	}
	for _, tagID := range appliedTagIDs {
		t, ok := byID[tagID]
		if !ok {
			continue
		}
		if t.SLAPolicyID.Valid && t.SLAPolicyID.Int != conv.SLAPolicyID.Int {
			if err := a.ConversationMgr.ApplySLA(conv, int(t.SLAPolicyID.Int), a.SystemUser); err != nil {
				a.Lo.Warn("apply tag SLA failed", "tag_id", tagID, "sla_policy_id", t.SLAPolicyID.Int, "error", err)
			}
		}
		if t.PriorityID.Valid && t.PriorityID.Int != conv.PriorityID.Int {
			if err := a.ConversationMgr.UpdateConversationPriority(conv.UUID, int(t.PriorityID.Int), "", a.SystemUser); err != nil {
				a.Lo.Warn("apply tag priority failed", "tag_id", tagID, "priority_id", t.PriorityID.Int, "error", err)
			}
		}
	}
}

func (a *Activities) recordAIUsage(ctx context.Context, convID int, traceID string, r Result, latencyMs int) {
	conversationID := int64(convID)
	row := usage.Row{
		CreatedAt:           time.Now().UTC(),
		TraceID:             traceID,
		CallSequenceInTrace: 1,
		InboxID:             a.SystemInboxID,
		ConversationID:      &conversationID,
		Provider:            "anthropic",
		Model:               r.Model,
		PresetSlug:          coalesce(r.PresetUsed, "fast-haiku"),
		Purpose:             string(llm.PurposeClassification),
		StopReason:          r.StopReason,
		InputTokens:         r.InputTokens,
		OutputTokens:        r.OutputTokens,
		CachedInputTokens:   r.CachedInputTokens,
		CacheMarkersUsed:    r.CacheMarkersUsed,
		LatencyMs:           latencyMs,
		RetryCount:          r.RetryCount,
		Outcome:             usage.OutcomeSuccess,
		ProviderMetadata:    r.ProviderMetadata,
	}
	if r.ProviderRequestID != "" {
		row.ProviderRequestID = &r.ProviderRequestID
	}
	if err := a.UsageMgr.Record(ctx, row); err != nil {
		a.Lo.Warn("tag classification ai_usage write failed", "trace_id", traceID, "error", err)
	}
}

func intSet(ids []int) map[int]bool {
	out := map[int]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func coalesce(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
