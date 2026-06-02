package capabilities

import (
	"context"
	"fmt"

	"github.com/abhinavxd/libredesk/internal/ai/tools/builtin"
)

func init() {
	builtin.RegisterInternal(&getConversationHistoryCapability{})
}

// getConversationHistoryCapability lists the customer's recent past
// conversations so an agent can ground its reply in earlier context.
//
// v1 resolves the contact from the current conversation (or a supplied
// conversation_id) and uses
// conversation.Manager.GetContactPreviousConversations. Lookup by
// customer_email is deferred — the inbound pipeline always has a
// conversation in scope, and one entry-point keeps the capability simple.
type getConversationHistoryCapability struct{}

func (g *getConversationHistoryCapability) ID() string { return "get_conversation_history" }
func (g *getConversationHistoryCapability) Name() string {
	return "List the customer's past conversations"
}

func (g *getConversationHistoryCapability) Description() string {
	return "Fetch PRIOR conversations from the same customer — different tickets from earlier interactions, NOT the current conversation. " +
		"The current ticket's prior messages are already available in your context (provided via the system message under \"## Conversation history (current ticket)\"); do NOT call this capability to read them.\n\n" +
		"Call this capability only when:\n" +
		"  - the customer explicitly references something from a previous interaction (\"as I mentioned last time\", \"I wrote two weeks ago\", \"wie letztes Mal\", \"vor zwei Wochen habe ich geschrieben\"); OR\n" +
		"  - older-ticket context would meaningfully change the response (the customer's pattern of complaints, an unresolved issue from a past ticket, etc.).\n\n" +
		"Returns up to `limit` past conversation summaries (most recent first) with conversation id, subject, last-message timestamp, and a snippet of the last message. " +
		"By default resolves the customer from the conversation currently in scope; pass `conversation_id` to resolve a different one explicitly."
}

func (g *getConversationHistoryCapability) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "integer",
				"description": "Conversation id to resolve the customer from. Defaults to the conversation currently in scope.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum past conversations to return (1-50; default 5).",
				"minimum":     1,
				"maximum":     50,
			},
		},
	}
}

func (g *getConversationHistoryCapability) OutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contact_id": map[string]any{"type": "integer"},
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"conversation_id": map[string]any{"type": "integer"},
						"uuid":            map[string]any{"type": "string"},
						"subject":         map[string]any{"type": "string"},
						"last_message_at": map[string]any{"type": "string"},
						"last_message":    map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func (g *getConversationHistoryCapability) Category() string { return "read" }

// ConversationHistoryEntry is the per-conversation shape returned. Subset
// of conversation/models.PreviousConversation tailored for LLM consumption.
type ConversationHistoryEntry struct {
	ConversationID int    `json:"conversation_id"`
	UUID           string `json:"uuid"`
	Subject        string `json:"subject,omitempty"`
	LastMessage    string `json:"last_message,omitempty"`
	LastMessageAt  string `json:"last_message_at,omitempty"`
}

// ConversationHistoryOutput is the envelope returned to the LLM.
type ConversationHistoryOutput struct {
	ContactID int                        `json:"contact_id"`
	Results   []ConversationHistoryEntry `json:"results"`
}

func (g *getConversationHistoryCapability) Execute(ctx context.Context, params map[string]any, rt *builtin.ToolRuntimeContext) (any, error) {
	if rt == nil || rt.Conversation == nil {
		return nil, fmt.Errorf("get_conversation_history: runtime conversation manager is nil")
	}

	convID, ok := paramInt(params, "conversation_id")
	if !ok || convID <= 0 {
		convID = rt.ConversationID
	}
	if convID <= 0 {
		return nil, fmt.Errorf("get_conversation_history: conversation_id is required (no conversation in scope and none provided)")
	}

	limit := 5
	if v, ok := paramInt(params, "limit"); ok {
		if v < 1 || v > 50 {
			return nil, fmt.Errorf("get_conversation_history: limit must be between 1 and 50 (got %d)", v)
		}
		limit = v
	}

	conv, err := rt.Conversation.GetConversation(convID, "", "")
	if err != nil {
		return nil, fmt.Errorf("get_conversation_history: %w", err)
	}
	if conv.ContactID <= 0 {
		return nil, fmt.Errorf("get_conversation_history: conversation %d has no contact_id", convID)
	}

	prev, err := rt.Conversation.GetContactPreviousConversations(conv.ContactID, limit)
	if err != nil {
		return nil, fmt.Errorf("get_conversation_history: %w", err)
	}

	results := make([]ConversationHistoryEntry, 0, len(prev))
	for _, p := range prev {
		// Skip the conversation we resolved from — the LLM is already
		// inside it; surfacing it again is noise.
		if p.ID == convID {
			continue
		}
		entry := ConversationHistoryEntry{
			ConversationID: p.ID,
			UUID:           p.UUID,
			Subject:        p.Subject,
		}
		if p.LastMessage.Valid {
			entry.LastMessage = p.LastMessage.String
		}
		if p.LastMessageAt.Valid {
			entry.LastMessageAt = p.LastMessageAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		results = append(results, entry)
	}

	return ConversationHistoryOutput{ContactID: conv.ContactID, Results: results}, nil
}
