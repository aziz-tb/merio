package runner

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// EventPublisher is the runner's progress-event sink.
//
// Implementations MUST be goroutine-safe: parallel read-tool dispatch
// may emit tool events concurrently within a single run.
//
// Publish failures are best-effort only. The runner logs them but never
// fails the underlying run because of a live-progress transport error.
type EventPublisher interface {
	Publish(ctx context.Context, traceID string, event RunEvent) error
}

// RunEvent is one progress update published during an agent run.
type RunEvent struct {
	Type      string          `json:"type"`
	EmittedAt time.Time       `json:"emitted_at"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

const (
	EventTypeRunEnqueued    = "run.enqueued"
	EventTypeRunStarted     = "run.started"
	EventTypeTurnStarted    = "turn.started"
	EventTypeTurnCompleted  = "turn.completed"
	EventTypeToolDispatched = "tool.dispatched"
	EventTypeToolCompleted  = "tool.completed"
	EventTypeDraftCommitted = "draft.committed"
	EventTypeRunCompleted   = "run.completed"
	EventTypeRunFailed      = "run.failed"
)

type RunEnqueuedPayload struct {
	Source         string `json:"source"`
	AgentID        int    `json:"agent_id"`
	ConversationID int    `json:"conversation_id"`
}

type RunStartedPayload struct {
	AgentID        int    `json:"agent_id"`
	AgentSlug      string `json:"agent_slug"`
	ConversationID int    `json:"conversation_id"`
}

type TurnStartedPayload struct {
	TurnIndex int `json:"turn_index"`
}

type TurnCompletedPayload struct {
	TurnIndex  int    `json:"turn_index"`
	StopReason string `json:"stop_reason"`
	LatencyMs  int    `json:"latency_ms"`
}

type ToolDispatchedPayload struct {
	TurnIndex int            `json:"turn_index"`
	ToolID    string         `json:"tool_id"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolKind  string         `json:"tool_kind,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

type ToolCompletedPayload struct {
	TurnIndex  int    `json:"turn_index"`
	ToolID     string `json:"tool_id"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolKind   string `json:"tool_kind,omitempty"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Data       any    `json:"data,omitempty"`
	Error      string `json:"error,omitempty"`
}

type DraftCommittedPayload struct {
	TurnIndex int `json:"turn_index"`
}

type RunCompletedPayload struct {
	ExecutionID int64        `json:"execution_id"`
	Source      string       `json:"source"`
	Status      RunStatus    `json:"status"`
	Response    *RunResponse `json:"response,omitempty"`
}

type RunFailedPayload struct {
	ExecutionID *int64    `json:"execution_id,omitempty"`
	Source      string    `json:"source"`
	Status      RunStatus `json:"status"`
	Error       string    `json:"error"`
}

func (r *Runner) eventPublisher(req RunRequest) EventPublisher {
	if req.EventPublisher != nil {
		return req.EventPublisher
	}
	return r.opts.Events
}

func (r *Runner) emit(ctx context.Context, traceID, eventType string, payload any, pub EventPublisher) {
	if pub == nil || strings.TrimSpace(traceID) == "" {
		return
	}

	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			if r.opts.Lo != nil {
				r.opts.Lo.Warn("runner.event.marshal_failed",
					"trace_id", traceID,
					"type", eventType,
					"error", err,
				)
			}
			return
		}
		raw = b
	}

	event := RunEvent{
		Type:      eventType,
		EmittedAt: time.Now().UTC(),
		Payload:   raw,
	}
	if err := pub.Publish(ctx, traceID, event); err != nil && r.opts.Lo != nil {
		r.opts.Lo.Warn("runner.event.publish_failed",
			"trace_id", traceID,
			"type", eventType,
			"error", err,
		)
	}
}
