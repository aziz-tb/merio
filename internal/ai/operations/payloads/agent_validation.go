package payloads

import "encoding/json"

// AgentValidationPayload is the operation-specific data for a single
// agent_validation operation. The composer reads MatchingAgentIDs to split
// matching agents from the ad-hoc searchable catalog.
type AgentValidationPayload struct {
	MatchingAgentIDs      []int                            `json:"matching_agent_ids"`
	PerAgent              map[string]AgentValidationDetail `json:"per_agent,omitempty"`
	AllConsideredAgentIDs []int                            `json:"all_considered_agent_ids"`
}

// AgentValidationDetail captures the evaluation result for one agent.
type AgentValidationDetail struct {
	Matched    bool    `json:"matched"`
	Confidence float64 `json:"confidence,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// Marshal renders the payload to JSON for ai_operations.payload.
func (p AgentValidationPayload) Marshal() (json.RawMessage, error) {
	return json.Marshal(p)
}

// UnmarshalAgentValidation parses an ai_operations.payload back into the typed shape;
// an empty payload (failed/skipped op) yields the zero value.
func UnmarshalAgentValidation(raw json.RawMessage) (AgentValidationPayload, error) {
	var p AgentValidationPayload
	if len(raw) == 0 {
		return p, nil
	}
	return p, json.Unmarshal(raw, &p)
}
