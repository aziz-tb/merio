package main

import (
	"github.com/abhinavxd/libredesk/internal/ai/tools/builtin"
	"github.com/zerodha/fastglue"
)

// internalCapabilityView is the public shape of one InternalCapability
// in list responses. Kept minimal — the agent edit form needs id,
// description, schema, and the legitimation gate; nothing else.
type internalCapabilityView struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	InputSchema     map[string]any `json:"input_schema"`
	OutputSchema    map[string]any `json:"output_schema,omitempty"`
	MinLegitimation int            `json:"min_legitimation"`
}

// handleListAIInternalCapabilities returns every registered internal
// capability sorted by id. Read-only, gated on ai_agents:manage (the
// only consumer is the agent edit form's tool picker; ai_tools:manage
// already grants browse access via its own list endpoint, so a single
// gate is enough).
//
// The registry is populated at startup and never mutates at runtime, so
// no caching/pagination is needed — the response is small and stable.
func handleListAIInternalCapabilities(r *fastglue.Request) error {
	caps := builtin.Default().List()
	out := make([]internalCapabilityView, 0, len(caps))
	for _, c := range caps {
		out = append(out, internalCapabilityView{
			ID:              c.ID(),
			Name:            c.Name(),
			Description:     c.Description(),
			InputSchema:     c.InputSchema(),
			OutputSchema:    c.OutputSchema(),
			MinLegitimation: c.MinLegitimation(),
		})
	}
	return r.SendEnvelope(out)
}
