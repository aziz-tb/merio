package capabilities

import (
	"context"
	"fmt"

	"github.com/abhinavxd/libredesk/internal/ai/tools/builtin"
)

func init() {
	builtin.RegisterInternal(&getCustomerDataCapability{})
}

// getCustomerDataCapability returns the contact attached to a
// conversation. PII fields (phone, custom attributes, country) are
// redacted unless the caller's legitimation level is at least
// piiLegitimationLevel; the agent runtime sets that level from the
// conversation's legitimation state in the inbound pipeline (F3+).
type getCustomerDataCapability struct{}

const piiLegitimationLevel = 1

func (g *getCustomerDataCapability) ID() string   { return "get_customer_data" }
func (g *getCustomerDataCapability) Name() string { return "Get the conversation's customer" }

func (g *getCustomerDataCapability) Description() string {
	return "Fetch the contact (customer) attached to a conversation. " +
		"Returns the contact's id, first and last name, email, account status, and timezone. " +
		"PII fields (phone number, country, custom attributes) are returned only when the caller's legitimation level is 1 or higher.\n\n" +
		"Use this to address the customer by name, check whether their account is enabled, " +
		"or look up details needed to draft an accurate reply. " +
		"By default resolves the contact for the conversation currently in scope; pass `conversation_id` to look up a different conversation explicitly."
}

func (g *getCustomerDataCapability) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_id": map[string]any{
				"type":        "integer",
				"description": "Conversation id to resolve the contact from. Defaults to the conversation currently in scope.",
			},
		},
	}
}

func (g *getCustomerDataCapability) OutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contact_id":         map[string]any{"type": "integer"},
			"first_name":         map[string]any{"type": "string"},
			"last_name":          map[string]any{"type": "string"},
			"email":              map[string]any{"type": "string"},
			"type":               map[string]any{"type": "string"},
			"enabled":            map[string]any{"type": "boolean"},
			"phone_number":       map[string]any{"type": "string"},
			"phone_country_code": map[string]any{"type": "string"},
			"country":            map[string]any{"type": "string"},
			"custom_attributes":  map[string]any{"type": "object"},
			"pii_redacted":       map[string]any{"type": "boolean"},
			"created_at":         map[string]any{"type": "string"},
		},
	}
}

func (g *getCustomerDataCapability) Category() string { return "read" }

func (g *getCustomerDataCapability) MinLegitimation() int { return 0 }

// CustomerData is the typed envelope returned by this capability. PII
// fields are populated only when the caller's legitimation level is high
// enough; pii_redacted=true on the response when they were dropped.
type CustomerData struct {
	ContactID        int            `json:"contact_id"`
	FirstName        string         `json:"first_name"`
	LastName         string         `json:"last_name"`
	Email            string         `json:"email,omitempty"`
	Type             string         `json:"type"`
	Enabled          bool           `json:"enabled"`
	PhoneNumber      string         `json:"phone_number,omitempty"`
	PhoneCountryCode string         `json:"phone_country_code,omitempty"`
	Country          string         `json:"country,omitempty"`
	CustomAttributes map[string]any `json:"custom_attributes,omitempty"`
	PIIRedacted      bool           `json:"pii_redacted"`
	CreatedAt        string         `json:"created_at,omitempty"`
}

func (g *getCustomerDataCapability) Execute(ctx context.Context, params map[string]any, rt *builtin.ToolRuntimeContext) (any, error) {
	if rt == nil || rt.Conversation == nil || rt.User == nil {
		return nil, fmt.Errorf("get_customer_data: runtime conversation/user managers are nil")
	}

	convID, ok := paramInt(params, "conversation_id")
	if !ok || convID <= 0 {
		convID = rt.ConversationID
	}
	if convID <= 0 {
		return nil, fmt.Errorf("get_customer_data: conversation_id is required (no conversation in scope and none provided)")
	}

	conv, err := rt.Conversation.GetConversation(convID, "", "")
	if err != nil {
		return nil, fmt.Errorf("get_customer_data: %w", err)
	}
	if conv.ContactID <= 0 {
		return nil, fmt.Errorf("get_customer_data: conversation %d has no contact_id", convID)
	}

	u, err := rt.User.GetContactOrVisitor(conv.ContactID, "")
	if err != nil {
		return nil, fmt.Errorf("get_customer_data: %w", err)
	}

	out := CustomerData{
		ContactID: u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Type:      u.Type,
		Enabled:   u.Enabled,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if u.Email.Valid {
		out.Email = u.Email.String
	}

	if rt.LegitimationLevel >= piiLegitimationLevel {
		if u.PhoneNumber.Valid {
			out.PhoneNumber = u.PhoneNumber.String
		}
		if u.PhoneNumberCountryCode.Valid {
			out.PhoneCountryCode = u.PhoneNumberCountryCode.String
		}
		if u.Country.Valid {
			out.Country = u.Country.String
		}
		if len(u.CustomAttributes) > 0 {
			// CustomAttributes is json.RawMessage; the LLM gets a parsed
			// map for free given the JSON encoder will round-trip.
			out.CustomAttributes = decodeCustomAttributes(u.CustomAttributes)
		}
	} else {
		out.PIIRedacted = true
	}

	return out, nil
}
