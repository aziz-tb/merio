// Package payloads defines the typed payload shapes persisted on ai_operations records.
package payloads

import "encoding/json"

// CustomerIdentificationPayload is the ai_operations.payload shape for
// operation_type = "customer_identification".
type CustomerIdentificationPayload struct {
	Method                 string                     `json:"method"`
	Channel                string                     `json:"channel"`
	SkipReason             string                     `json:"skip_reason,omitempty"`
	PatternDetection       any                        `json:"pattern_detection,omitempty"`
	LLMExtraction          any                        `json:"llm_extraction,omitempty"`
	SearchCustomerByFields any                        `json:"search_customer_by_fields,omitempty"`
	Load                   any                        `json:"load,omitempty"`
	Verification           CustomerVerificationResult `json:"verification"`
	Result                 CustomerIdentificationLink `json:"result"`
}

// CustomerVerificationResult is the outcome of verifying a candidate customer match.
type CustomerVerificationResult struct {
	Level        int      `json:"level"`
	Source       string   `json:"source"`
	Message      *string  `json:"message,omitempty"`
	RulesMatched []string `json:"rules_matched,omitempty"`
	RulesFailed  []string `json:"rules_failed,omitempty"`
}

// CustomerIdentificationLink is the resolved link to a customer and contract, with the
// previous link it superseded (if any).
type CustomerIdentificationLink struct {
	CustomerID         *string `json:"customer_id,omitempty"`
	ContractID         *string `json:"contract_id,omitempty"`
	PreviousCustomerID *string `json:"previous_customer_id,omitempty"`
	PreviousContractID *string `json:"previous_contract_id,omitempty"`
}

// Marshal renders the payload to JSON for ai_operations.payload.
func (p CustomerIdentificationPayload) Marshal() (json.RawMessage, error) {
	return json.Marshal(p)
}

// UnmarshalCustomerIdentification parses an ai_operations.payload back into the typed
// shape; an empty payload (failed/skipped op) yields the zero value.
func UnmarshalCustomerIdentification(raw json.RawMessage) (CustomerIdentificationPayload, error) {
	var p CustomerIdentificationPayload
	if len(raw) == 0 {
		return p, nil
	}
	return p, json.Unmarshal(raw, &p)
}
