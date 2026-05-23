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

const (
	DefaultChannel = "libredesk:realtime"

	EventConversationUpdate       = "conversation_update"
	EventAgentValidationCompleted = "agent_validation_completed"
)

type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Publisher interface {
	Publish(context.Context, Event) error
}

type Handler func(context.Context, Event) error

func NewEvent(eventType string, data any) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	return Event{Type: eventType, Data: b}, nil
}
