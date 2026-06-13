// Package realtime carries ephemeral UI update events from background workers
// to API processes, where they are forwarded to connected WebSocket clients.
//
// Events published here are not durable and must never be used as the source
// of truth for business logic. Persist state in Postgres or Temporal first,
// then publish a realtime event to refresh the UI.
package realtime

import (
	"context"
	"encoding/json"
)

// DefaultChannel is the Redis pub/sub channel for realtime events; the Event* constants
// are the event-type names carried in Event.Type.
const (
	DefaultChannel = "merio:realtime"

	EventConversationUpdate              = "conversation_update"
	EventAgentValidationCompleted        = "agent_validation_completed"
	EventTagClassificationCompleted      = "tag_classification_completed"
	EventCustomerIdentificationCompleted = "customer_identification_completed"
)

// Event is a realtime UI event: a Type and an opaque JSON Data payload.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Publisher publishes realtime events (to Redis, or a no-op when realtime is disabled).
type Publisher interface {
	Publish(context.Context, Event) error
}

// Handler processes a received realtime Event.
type Handler func(context.Context, Event) error

// WorkflowCompletedEvent is the payload for a completed background workflow
// (agent validation, tag classification, or customer identification).
type WorkflowCompletedEvent struct {
	Type                     string  `json:"type"`
	ConversationUUID         string  `json:"conversation_uuid"`
	Status                   string  `json:"status"`
	TraceID                  string  `json:"trace_id"`
	Error                    string  `json:"error"`
	PreviousLinkedCustomerID *string `json:"previous_linked_customer_id,omitempty"`
}

// NewEvent builds an Event of the given type by JSON-encoding data as the payload.
func NewEvent(eventType string, data any) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	return Event{Type: eventType, Data: b}, nil
}
