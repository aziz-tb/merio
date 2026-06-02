package main

import (
	"errors"
	"time"

	aiexecutions "github.com/aziz-tb/merio/internal/ai/executions"
	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func handleConversationAgentRunState(r *fastglue.Request) error {
	app := r.Context.(*App)
	uuid, _ := r.RequestCtx.UserValue("uuid").(string)
	if uuid == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	conv, err := app.conversation.GetConversation(0, uuid, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if app.temporal != nil {
		active, err := findRunningAgentRun(r.RequestCtx, app.temporal, conv.ID, nil)
		if err != nil {
			app.lo.Error("failed to query in-flight agent run state",
				"conversation_id", conv.ID,
				"error", err)
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
				app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		if active != nil {
			return r.SendEnvelope(map[string]any{
				"state":      "in_flight",
				"trace_id":   active.TraceID,
				"agent_id":   active.AgentID,
				"started_at": active.StartedAt,
			})
		}
	}

	exec, err := app.aiExecutions.LatestExecutionForConversation(int64(conv.ID))
	if err != nil {
		if errors.Is(err, aiexecutions.ErrNotFound) {
			return r.SendEnvelope(map[string]any{"state": "none"})
		}
		return sendErrorEnvelope(r, err)
	}
	if time.Since(exec.RunAt) <= recentCompletionWindow() {
		return r.SendEnvelope(map[string]any{
			"state":        "completed",
			"trace_id":     exec.TraceID,
			"execution_id": exec.ID,
			"completed_at": exec.RunAt,
			"status":       exec.Status,
		})
	}
	return r.SendEnvelope(map[string]any{"state": "none"})
}

func recentCompletionWindow() time.Duration {
	if d := ko.Duration("agent_run.recent_completion_window"); d > 0 {
		return d
	}
	return 10 * time.Minute
}
