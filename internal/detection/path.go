package detection

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
)

// ResolvePath returns the value at the source/path in ctx.
func ResolvePath(ctx EvaluationContext, source, path string) (any, bool) {
	switch source {
	case SourceTags:
		return ctx.Tags, true
	case SourceTicket:
		ticketJSON, err := BuildTicketJSON(ctx.Conversation)
		if err != nil {
			return nil, false
		}
		return navigatePath(ticketJSON, parsePath(path))
	case SourceContact:
		contactJSON, err := BuildContactJSON(ctx.Conversation.Contact)
		if err != nil {
			return nil, false
		}
		return navigatePath(contactJSON, parsePath(path))
	}
	return nil, false
}

// BuildTicketJSON marshals a conversation and injects synthetic computed fields.
func BuildTicketJSON(conv cmodels.Conversation) (map[string]any, error) {
	raw, err := json.Marshal(conv)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["computed"] = ComputeTicketFields(conv)
	rehydrateRawJSONFields(m, []string{"meta", "custom_attributes"})
	return m, nil
}

// BuildContactJSON marshals a conversation contact into a navigable map.
func BuildContactJSON(contact cmodels.ConversationContact) (map[string]any, error) {
	raw, err := json.Marshal(contact)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	rehydrateRawJSONFields(m, []string{"custom_attributes"})
	return m, nil
}

// ComputeTicketFields returns synthetic time-based fields exposed at
// ticket.computed.*. Values are float64 hours; missing timestamps resolve to nil.
func ComputeTicketFields(conv cmodels.Conversation) map[string]any {
	now := time.Now()
	out := map[string]any{
		"hours_since_created": now.Sub(conv.CreatedAt).Hours(),
	}

	if !conv.FirstReplyAt.IsZero() {
		out["hours_since_first_reply"] = now.Sub(conv.FirstReplyAt.Time).Hours()
	} else {
		out["hours_since_first_reply"] = nil
	}

	if !conv.LastReplyAt.IsZero() {
		out["hours_since_last_reply"] = now.Sub(conv.LastReplyAt.Time).Hours()
	} else {
		out["hours_since_last_reply"] = nil
	}

	if !conv.ResolvedAt.IsZero() {
		out["hours_since_resolved"] = now.Sub(conv.ResolvedAt.Time).Hours()
	} else {
		out["hours_since_resolved"] = nil
	}

	return out
}

func rehydrateRawJSONFields(m map[string]any, keys []string) {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		strVal, ok := v.(string)
		if !ok || strVal == "" {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
			m[key] = parsed
		}
	}
}

func parsePath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func navigatePath(root any, segments []string) (any, bool) {
	current := root
	for _, seg := range segments {
		switch v := current.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			current = v[idx]
		default:
			return nil, false
		}
	}
	return current, true
}
