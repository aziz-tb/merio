package anthropic

import (
	"errors"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/zerodha/logf"

	"github.com/aziz-tb/merio/internal/ai/llm"
)

// callLog captures a single completion call's structured log fields.
// Materialized once so emit() can write a single line on success and on
// failure paths.
type callLog struct {
	startedAt    time.Time
	traceID      string
	purpose      string
	preset       string
	model        string
	agentID      int
	resAttemptID int
	cacheMarkers int
	attempts     int
}

func (c *callLog) emitSuccess(lo *logf.Logger, resp llm.CompletionResponse) {
	lo.Info("anthropic.complete",
		"trace_id", c.traceID,
		"purpose", c.purpose,
		"preset", c.preset,
		"model", c.model,
		"agent_id", c.agentID,
		"resolution_attempt_id", c.resAttemptID,
		"latency_ms", int(time.Since(c.startedAt).Milliseconds()),
		"input_tokens", resp.Usage.InputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"cached_tokens", resp.Usage.CachedTokens,
		"cache_markers", c.cacheMarkers,
		"stop_reason", string(resp.StopReason),
		"attempts", c.attempts,
		"outcome", "success",
	)
}

func (c *callLog) emitFailure(lo *logf.Logger, err error) {
	// Extract Anthropic's structured error details when available
	apiStatusCode := 0
	apiErrorMessage := ""
	apiRequestID := ""
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		apiStatusCode = apiErr.StatusCode
		apiErrorMessage = apiErr.Error() // includes serialized response body
		apiRequestID = apiErr.RequestID
	}

	lo.Error("anthropic.complete",
		"trace_id", c.traceID,
		"purpose", c.purpose,
		"preset", c.preset,
		"model", c.model,
		"agent_id", c.agentID,
		"resolution_attempt_id", c.resAttemptID,
		"latency_ms", int(time.Since(c.startedAt).Milliseconds()),
		"cache_markers", c.cacheMarkers,
		"attempts", c.attempts,
		"outcome", "error",
		"error", err.Error(),
		"api_status_code", apiStatusCode,
		"api_error_message", apiErrorMessage,
		"api_request_id", apiRequestID,
	)
}
