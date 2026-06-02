package conversation

import (
	"context"
	"sort"

	"github.com/aziz-tb/merio/internal/conversation/models"
)

// LoadMessagesForLLM returns the public, non-activity messages for a
// conversation, sorted oldest → newest. Returns up to 500 messages
// (GetConversationMessages' max page size).
//
// This is the shared substrate for LLM-facing consumers (tag
// classification today; agent context-building, future ops thereafter).
// Each consumer applies its own formatting and capping on top — this
// helper does NOT format, truncate, strip HTML, or limit by message count;
// those are per-consumer decisions.
func (m *Manager) LoadMessagesForLLM(ctx context.Context, conversationUUID string) ([]models.Message, error) {
	_ = ctx
	notPrivate := false
	msgs, _, err := m.GetConversationMessages(conversationUUID, 1, 500, &notPrivate, nil)
	if err != nil {
		return nil, err
	}
	return filterMessagesForLLM(msgs), nil
}

// filterMessagesForLLM drops activity messages and sorts oldest → newest.
// Pure (no DB) so the filter/sort behavior is unit-testable. Private
// messages are already excluded by the GetConversationMessages query.
func filterMessagesForLLM(msgs []models.Message) []models.Message {
	filtered := make([]models.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Type == models.MessageActivity {
			continue
		}
		filtered = append(filtered, msg)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})

	return filtered
}
