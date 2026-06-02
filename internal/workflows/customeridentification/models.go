package customeridentification

import (
	"encoding/json"

	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	customermodels "github.com/aziz-tb/merio/internal/customer/models"
	verificationmodels "github.com/aziz-tb/merio/internal/verification/models"
)

const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusNoMatch = "no_match"
	StatusSkipped = "skipped"

	MethodPattern = "pattern"
	MethodLLM     = "llm"
	MethodManual  = "manual"
	MethodNone    = "none"
)

type WorkflowInput struct {
	ConversationID    int64  `json:"conversation_id"`
	MessageID         int64  `json:"message_id"`
	Channel           string `json:"channel"`
	TriggeredByUserID *int64 `json:"triggered_by_user_id,omitempty"`
	TraceID           string `json:"trace_id"`
}

type WorkflowOutput struct {
	Status                   string  `json:"status"`
	CustomerID               *string `json:"customer_id,omitempty"`
	ContractID               *string `json:"contract_id,omitempty"`
	VerificationLevel        int     `json:"verification_level"`
	VerificationMessage      string  `json:"verification_message,omitempty"`
	AIOperationID            int64   `json:"ai_operation_id"`
	PreviousLinkedCustomerID *string `json:"previous_linked_customer_id,omitempty"`
}

type Settings struct {
	PatternDetectionEnabled bool                          `json:"pattern_detection_enabled"`
	LLMExtractionEnabled    bool                          `json:"llm_extraction_enabled"`
	ContractNumberRegex     string                        `json:"contract_number_regex"`
	CustomerNumberRegex     string                        `json:"customer_number_regex"`
	ExcludedEmailDomains    []string                      `json:"excluded_email_domains"`
	SearchParameters        map[string]SearchParameter    `json:"search_parameters"`
	VerificationRulesAsync  verificationmodels.RulesAsync `json:"verification_rules_async"`
	ERPCacheTTLMinutes      int                           `json:"erp_cache_ttl_minutes"`
}

type SearchParameter struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type LoadSettingsInput struct{}

type CreateOperationInput struct {
	ConversationID    int64  `json:"conversation_id"`
	Channel           string `json:"channel"`
	TriggeredByUserID *int64 `json:"triggered_by_user_id,omitempty"`
	TraceID           string `json:"trace_id"`
}

type CreateOperationOutput struct {
	AIOperationID int64 `json:"ai_operation_id"`
}

type ShouldRunInput struct {
	ConversationID int64    `json:"conversation_id"`
	Channel        string   `json:"channel"`
	Settings       Settings `json:"settings"`
}

type ShouldRunOutput struct {
	ShouldRun bool   `json:"should_run"`
	Reason    string `json:"reason,omitempty"`
}

type PrepareMessageInput struct {
	ConversationID int64    `json:"conversation_id"`
	MessageID      int64    `json:"message_id"`
	Settings       Settings `json:"settings"`
}

type PrepareMessageOutput struct {
	PreparedMessage convmodels.PreparedMessage `json:"prepared_message"`
}

type PatternDetectInput struct {
	PreparedMessage convmodels.PreparedMessage `json:"prepared_message"`
	Settings        Settings                   `json:"settings"`
}

type CachedMatch struct {
	Kind       string  `json:"kind"`
	Email      string  `json:"email"`
	CustomerID *string `json:"customer_id,omitempty"`
	ContractID *string `json:"contract_id,omitempty"`
	EntityID   string  `json:"entity_id,omitempty"`
}

type PatternDetectOutput struct {
	Ran                 bool           `json:"ran"`
	ContractNumberMatch *string        `json:"contract_number_match,omitempty"`
	CustomerNumberMatch *string        `json:"customer_number_match,omitempty"`
	SenderEmailMatch    *CachedMatch   `json:"sender_email_match,omitempty"`
	BodyEmailMatches    []CachedMatch  `json:"body_email_matches,omitempty"`
	ResolvedContractID  *string        `json:"resolved_contract_id,omitempty"`
	ExtractedAttrs      map[string]any `json:"extracted_attrs,omitempty"`
}

type LLMExtractInput struct {
	ConversationID  int64                      `json:"conversation_id"`
	TraceID         string                     `json:"trace_id"`
	Channel         string                     `json:"channel"`
	PreparedMessage convmodels.PreparedMessage `json:"prepared_message"`
	Settings        Settings                   `json:"settings"`
	PatternAttrs    map[string]any             `json:"pattern_attrs,omitempty"`
}

type LLMUsage struct {
	Provider          string          `json:"provider,omitempty"`
	Model             string          `json:"model,omitempty"`
	PresetUsed        string          `json:"preset_used,omitempty"`
	StopReason        string          `json:"stop_reason,omitempty"`
	InputTokens       int             `json:"input_tokens,omitempty"`
	OutputTokens      int             `json:"output_tokens,omitempty"`
	CachedInputTokens int             `json:"cached_input_tokens,omitempty"`
	CacheMarkersUsed  int             `json:"cache_markers_used,omitempty"`
	LatencyMs         int             `json:"latency_ms,omitempty"`
	RetryCount        int             `json:"retry_count,omitempty"`
	ProviderRequestID string          `json:"provider_request_id,omitempty"`
	ProviderMetadata  json.RawMessage `json:"provider_metadata,omitempty"`
}

type LLMExtractOutput struct {
	Ran            bool           `json:"ran"`
	ExtractedAttrs map[string]any `json:"extracted_attrs,omitempty"`
	Usage          LLMUsage       `json:"usage,omitempty"`
}

type SearchCustomerByFieldsInput struct {
	ConversationID    int64          `json:"conversation_id"`
	TriggeredByUserID *int64         `json:"triggered_by_user_id,omitempty"`
	TraceID           string         `json:"trace_id"`
	Attrs             map[string]any `json:"attrs"`
}

type SearchCustomerByFieldsOutput struct {
	Input                            map[string]any  `json:"input,omitempty"`
	Output                           map[string]any  `json:"output,omitempty"`
	ContractID                       *string         `json:"contract_id,omitempty"`
	CustomerID                       *string         `json:"customer_id,omitempty"`
	AgentPreview                     json.RawMessage `json:"agent_preview,omitempty"`
	CustomerVerification             *int            `json:"customer_verification,omitempty"`
	CustomerVerificationMessage      *string         `json:"customer_verification_message,omitempty"`
	MultipleMatchesContractViolation bool            `json:"multiple_matches_contract_violation,omitempty"`
}

type LoadContractAndCustomerInput struct {
	ConversationID    int64   `json:"conversation_id"`
	TriggeredByUserID *int64  `json:"triggered_by_user_id,omitempty"`
	TraceID           string  `json:"trace_id"`
	ContractID        *string `json:"contract_id,omitempty"`
	CustomerID        *string `json:"customer_id,omitempty"`
}

type LoadContractAndCustomerOutput struct {
	Customer *customermodels.Customer `json:"customer,omitempty"`
	Contract *contractmodels.Contract `json:"contract,omitempty"`
}

type EvaluateVerificationInput struct {
	Channel                     string                        `json:"channel"`
	PreparedMessage             convmodels.PreparedMessage    `json:"prepared_message"`
	Customer                    *customermodels.Customer      `json:"customer,omitempty"`
	Contract                    *contractmodels.Contract      `json:"contract,omitempty"`
	ExtractedAttrs              map[string]any                `json:"extracted_attrs,omitempty"`
	Rules                       verificationmodels.RulesAsync `json:"rules"`
	CustomerVerification        *int                          `json:"customer_verification,omitempty"`
	CustomerVerificationMessage *string                       `json:"customer_verification_message,omitempty"`
}

type EvaluateVerificationOutput struct {
	Level   int                                  `json:"level"`
	Message string                               `json:"message,omitempty"`
	Source  string                               `json:"source"`
	Matched []verificationmodels.CriterionResult `json:"matched,omitempty"`
	Failed  []verificationmodels.CriterionResult `json:"failed,omitempty"`
}

type PersistResultInput struct {
	AIOperationID     int64                         `json:"ai_operation_id"`
	ConversationID    int64                         `json:"conversation_id"`
	Channel           string                        `json:"channel"`
	TriggeredByUserID *int64                        `json:"triggered_by_user_id,omitempty"`
	TraceID           string                        `json:"trace_id"`
	Status            string                        `json:"status"`
	Method            string                        `json:"method"`
	SkipReason        string                        `json:"skip_reason,omitempty"`
	CustomerID        *string                       `json:"customer_id,omitempty"`
	ContractID        *string                       `json:"contract_id,omitempty"`
	Verification      EvaluateVerificationOutput    `json:"verification"`
	PatternDetection  PatternDetectOutput           `json:"pattern_detection"`
	LLMExtraction     LLMExtractOutput              `json:"llm_extraction"`
	SearchByFields    SearchCustomerByFieldsOutput  `json:"search_by_fields"`
	Load              LoadContractAndCustomerOutput `json:"load"`
	ErrorMessage      string                        `json:"error_message,omitempty"`
}

type PersistResultOutput struct {
	AIOperationID            int64   `json:"ai_operation_id"`
	PreviousLinkedCustomerID *string `json:"previous_linked_customer_id,omitempty"`
}
