package payloads

import "encoding/json"

// TagClassificationPayload is the ai_operations.payload shape for
// operation_type = "tag_classification". Token counts / latency are NOT
// stored here — query ai_usage by the operation's trace_id for the
// underlying LLM call. Reasoning is display-only; nothing queries it.
type TagClassificationPayload struct {
	CandidateTagIDs []int              `json:"candidate_tag_ids"`
	AppliedTagIDs   []int              `json:"applied_tag_ids"`
	RemovedTagIDs   []int              `json:"removed_tag_ids"`
	LLMScores       map[string]float64 `json:"llm_scores,omitempty"`
	Reasoning       string             `json:"reasoning,omitempty"`
	LLMModel        string             `json:"llm_model,omitempty"`
}

// Marshal renders the payload to JSON for ai_operations.payload.
func (p TagClassificationPayload) Marshal() (json.RawMessage, error) {
	return json.Marshal(p)
}

// UnmarshalTagClassification parses an ai_operations.payload back into the
// typed shape. An empty payload (failed/skipped op) yields the zero value.
func UnmarshalTagClassification(raw json.RawMessage) (TagClassificationPayload, error) {
	var p TagClassificationPayload
	if len(raw) == 0 {
		return p, nil
	}
	err := json.Unmarshal(raw, &p)
	return p, err
}
