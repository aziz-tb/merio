package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aziz-tb/merio/internal/ai/llm"
	aipreset "github.com/aziz-tb/merio/internal/ai/llm/preset"
	aiusage "github.com/aziz-tb/merio/internal/ai/usage"
	authmodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type aiCompletionReq struct {
	PromptKey string `json:"prompt_key"`
	Content   string `json:"content"`
}

// handleAICompletion handles AI completion requests
func handleAICompletion(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = aiCompletionReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("errors.parsingRequest"), nil))
	}
	req.PromptKey = strings.TrimSpace(req.PromptKey)
	if req.PromptKey == "" {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.T("globals.messages.badRequest"), nil))
	}

	resp, err := runRewriteCompletion(app, req, rewriteUserIDFromRequest(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(resp)
}

func runRewriteCompletion(app *App, req aiCompletionReq, userID *int64) (string, error) {
	if app == nil {
		return "", envelope.NewErrorWithCode(envelope.GeneralError, fasthttp.StatusServiceUnavailable,
			"AI rewrite provider not initialized", nil)
	}
	if app.rewrite == nil || app.aiProvider == nil || app.aiPresets == nil {
		return "", envelope.NewErrorWithCode(envelope.GeneralError, fasthttp.StatusServiceUnavailable,
			app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	systemPrompt, err := app.rewrite.PromptContent(req.PromptKey)
	if err != nil {
		return "", err
	}
	pre, err := app.aiPresets.Resolve(llm.PurposeRewrite, nil)
	if err != nil {
		return "", envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}

	traceID := "rewrite:" + uuid.NewString()
	compReq := llm.CompletionRequest{
		Purpose:    llm.PurposeRewrite,
		PresetSlug: pre.Slug,
		TraceID:    traceID,
		Messages: []llm.Message{
			{
				Role: llm.RoleSystem,
				Content: []llm.ContentBlock{{
					Type: llm.ContentBlockTypeText,
					Text: systemPrompt,
				}},
			},
			{
				Role: llm.RoleUser,
				Content: []llm.ContentBlock{{
					Type: llm.ContentBlockTypeText,
					Text: req.Content,
				}},
			},
		},
	}

	started := time.Now()
	resp, callErr := app.aiProvider.Complete(context.Background(), compReq)
	latencyMs := int(time.Since(started).Milliseconds())
	recordRewriteUsage(context.Background(), app, req, compReq, pre, resp, traceID, latencyMs, userID, callErr)
	if callErr != nil {
		app.lo.Error("rewrite completion failed", "trace_id", traceID, "prompt_key", req.PromptKey, "error", callErr)
		return "", rewriteEnvelopeError(app, callErr)
	}

	text := strings.TrimSpace(llm.ExtractText(resp.Content))
	if text == "" {
		app.lo.Error("rewrite completion returned empty response", "trace_id", traceID, "prompt_key", req.PromptKey)
		return "", envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	return text, nil
}

func recordRewriteUsage(ctx context.Context, app *App, req aiCompletionReq, compReq llm.CompletionRequest, pre aipreset.Preset, resp llm.CompletionResponse, traceID string, latencyMs int, userID *int64, callErr error) {
	if app == nil || app.aiUsage == nil || app.systemInboxID <= 0 {
		return
	}

	row := aiusage.Row{
		CreatedAt:           time.Now().UTC(),
		TraceID:             traceID,
		CallSequenceInTrace: 1,
		InboxID:             app.systemInboxID,
		Provider:            rewriteCoalesce(resp.Provider, resp.Observability.Provider, pre.Provider),
		Model:               rewriteCoalesce(resp.Model, pre.Model),
		PresetSlug:          rewriteCoalesce(resp.PresetUsed, pre.Slug),
		Purpose:             string(llm.PurposeRewrite),
		StopReason:          string(resp.StopReason),
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CachedInputTokens:   resp.Usage.CachedTokens,
		CacheMarkersUsed:    resp.CacheMetrics.Markers,
		LatencyMs:           latencyMs,
		RetryCount:          resp.Observability.RetryCount,
		Outcome:             rewriteOutcomeForError(callErr),
		ProviderMetadata:    resp.Observability.ProviderMetadata,
		RequestPayload:      rewriteRequestPayload(req, compReq),
		ResponsePayload:     rewriteResponsePayload(resp),
	}
	if resp.Observability.ProviderRequestID != "" {
		id := resp.Observability.ProviderRequestID
		row.ProviderRequestID = &id
	}
	if callErr != nil {
		cls := rewriteErrorClass(callErr)
		msg := callErr.Error()
		row.ErrorClass = &cls
		row.ErrorMessage = &msg
	}
	if userID != nil {
		row.UserID = userID
	}
	if err := app.aiUsage.Record(ctx, row); err != nil {
		app.lo.Warn("rewrite ai_usage write failed", "trace_id", traceID, "error", err)
	}
}

func rewriteUserIDFromRequest(r *fastglue.Request) *int64 {
	if r == nil {
		return nil
	}
	user, ok := r.RequestCtx.UserValue("user").(authmodels.User)
	if !ok || user.ID <= 0 {
		return nil
	}
	id := int64(user.ID)
	return &id
}

func rewriteEnvelopeError(app *App, err error) error {
	switch {
	case errors.Is(err, llm.ErrAuth):
		return envelope.NewErrorWithCode(envelope.GeneralError, fasthttp.StatusServiceUnavailable,
			app.i18n.Ts("ai.apiKeyNotSet", "provider", "LLM"), nil)
	case errors.Is(err, llm.ErrRateLimited):
		return envelope.NewError(envelope.RateLimitError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	case errors.Is(err, llm.ErrCircuitOpen), errors.Is(err, llm.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return envelope.NewError(envelope.NetworkError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	default:
		return envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
}

func rewriteOutcomeForError(err error) string {
	if err == nil {
		return aiusage.OutcomeSuccess
	}
	switch {
	case errors.Is(err, llm.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return aiusage.OutcomeTimeout
	case errors.Is(err, llm.ErrAuth):
		return aiusage.OutcomeAuthError
	case errors.Is(err, llm.ErrRateLimited):
		return aiusage.OutcomeRateLimited
	case errors.Is(err, llm.ErrCircuitOpen):
		return aiusage.OutcomeCircuitOpen
	case errors.Is(err, llm.ErrInvalidRequest):
		return aiusage.OutcomeInvalidRequest
	default:
		return aiusage.OutcomeOther
	}
}

func rewriteErrorClass(err error) string {
	switch rewriteOutcomeForError(err) {
	case aiusage.OutcomeAuthError:
		return "auth_error"
	case aiusage.OutcomeRateLimited:
		return "rate_limited"
	case aiusage.OutcomeCircuitOpen:
		return "circuit_open"
	case aiusage.OutcomeInvalidRequest:
		return "invalid_request"
	case aiusage.OutcomeTimeout:
		return "timeout"
	default:
		return "other"
	}
}

func rewriteRequestPayload(req aiCompletionReq, compReq llm.CompletionRequest) json.RawMessage {
	return rewriteJSON(map[string]any{
		"purpose":       string(llm.PurposeRewrite),
		"preset_slug":   compReq.PresetSlug,
		"trace_id":      compReq.TraceID,
		"prompt_key":    req.PromptKey,
		"content_bytes": len([]byte(req.Content)),
		"content_chars": len([]rune(req.Content)),
	})
}

func rewriteResponsePayload(resp llm.CompletionResponse) json.RawMessage {
	if resp.Provider == "" && resp.Model == "" && len(resp.Content) == 0 && resp.StopReason == "" {
		return nil
	}
	return rewriteJSON(map[string]any{
		"provider":       resp.Provider,
		"model":          resp.Model,
		"preset_used":    resp.PresetUsed,
		"stop_reason":    string(resp.StopReason),
		"content_blocks": len(resp.Content),
		"text_chars":     len([]rune(llm.ExtractText(resp.Content))),
	})
}

func rewriteJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return raw
}

func rewriteCoalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// handleGetAIPrompts returns AI prompts
func handleGetAIPrompts(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	resp, err := app.rewrite.GetPrompts()
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(resp)
}
