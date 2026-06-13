// Package models holds the input/output and rule shapes for the verification
// engine, which evaluates a channel's configured rules against a message,
// resolved customer/contract, and extracted attributes to produce a level,
// reason, and per-criterion trace.
package models

import (
	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	custmodels "github.com/aziz-tb/merio/internal/customer/models"
)

// VerificationLevel is how confidently a conversation's contact has been identified.
type VerificationLevel int

// Verification levels, from unverified (0) to operator-set manual confirmation (3).
const (
	VerificationLevelUnverified VerificationLevel = 0
	VerificationLevelPartial    VerificationLevel = 1
	VerificationLevelVerified   VerificationLevel = 2
	VerificationLevelManual     VerificationLevel = 3
)

// EvalInput is what Manager.Evaluate receives.
type EvalInput struct {
	// Channel selects which ruleset under verification_rules_async applies.
	// "email" is the only async channel in v1.
	Channel string
	Message convmodels.PreparedMessage
	// Customer / Contract are the resolved cache records; either may be nil
	// when Activity 4 returned no match.
	Customer *custmodels.Customer
	Contract *contractmodels.Contract
	// ExtractedAttrs holds attributes from pattern detection + LLM
	// extraction, e.g. {"contractId":"12606412","firstname":"Moussa"}.
	ExtractedAttrs map[string]any
	// Rules are pre-loaded by the caller (from the
	// customer_identification.verification_rules_async setting); the engine has
	// no settings dependency of its own and selects Rules[Channel].
	Rules RulesAsync
}

// CriterionContext is the per-criterion evaluation context.
type CriterionContext struct {
	Message        convmodels.PreparedMessage
	Customer       *custmodels.Customer
	Contract       *contractmodels.Contract
	ExtractedAttrs map[string]any
}

// CriterionResult is one entry in the evaluation trace.
type CriterionResult struct {
	Type       string `json:"type"`            // "standard" | "custom"
	Name       string `json:"name,omitempty"`  // standard criterion name
	Label      string `json:"label,omitempty"` // custom criterion label
	GroupLabel string `json:"group_label"`
	Reason     string `json:"reason,omitempty"` // rendered failure reason (custom only)
}

// EvalOutput is the engine's result.
type EvalOutput struct {
	// Level is 0, 1, or 2 — never 3 (3 is operator-set manual
	// confirmation, not derived from rules).
	Level VerificationLevel `json:"level"`
	// Message is the operator-facing reason: empty at level 2; the
	// concatenated reasons of failed custom criteria in partially-satisfied
	// groups at level 1; empty at level 0.
	Message string `json:"message"`
	// Source is "rules" for this engine. The workflow sets "user_code" when
	// Activity 4's executor returned customerVerification, or "manual" for
	// operator overrides — those paths bypass this engine.
	Source  string            `json:"source"`
	Matched []CriterionResult `json:"matched"`
	Failed  []CriterionResult `json:"failed"`
}

// RulesAsync is the shape of customer_identification.verification_rules_async
// — keyed by channel.
type RulesAsync map[string]ChannelRules

// ByChannel returns the ruleset for a channel and whether one is configured.
func (r RulesAsync) ByChannel(channel string) (ChannelRules, bool) {
	ch, ok := r[channel]
	return ch, ok
}

// ChannelRules is one channel's ruleset.
type ChannelRules struct {
	Groups []Group `json:"groups"`
}

// Group is a set of criteria with a minimum match count.
type Group struct {
	Label          string            `json:"label"`
	MinCount       int               `json:"min_count"`
	Criteria       []RuleCriterion   `json:"criteria"`
	CustomCriteria []CustomCriterion `json:"custom_criteria"`
}

// RuleCriterion references a standard criterion by Name or a custom
// criterion by Label.
type RuleCriterion struct {
	Type  string `json:"type"`            // "standard" | "custom"
	Name  string `json:"name,omitempty"`  // standard
	Label string `json:"label,omitempty"` // custom
}

// CustomCriterion is one group-scoped custom criterion under
// customer_identification.verification_rules_async.*.groups[].custom_criteria.
type CustomCriterion struct {
	Label    string   `json:"label"`
	Key      string   `json:"key"`
	Reason   string   `json:"reason"`
	SearchIn []string `json:"search_in"`
}
