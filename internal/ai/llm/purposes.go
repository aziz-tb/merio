// Package llm defines the provider-agnostic shape of LLM interactions used
// across the AI runtime: request/response types, content blocks, tool
// definitions, and the LLMProvider interface. Provider implementations
// (Anthropic, OpenAI, Azure, fakes) live in subpackages and translate this
// shape to and from their wire format.
package llm

// Purpose enumerates the ways the platform invokes an LLM. Each resolves to
// a preset via config/ai_presets.json's purpose_mappings (with optional
// per-agent overrides in ai_agents.preset_overrides), and is the dimension
// ai_usage rows are bucketed on for cost dashboards.
//
// Adding a purpose: append the constant here, add KnownPurposes, and add a
// default mapping in config/ai_presets.json. The registry validates at
// startup that every purpose_mappings entry is in this set.
type Purpose string

const (
	// PurposeClassification — short structured-output decisions (spam, language,
	// tag classification). Latency-sensitive, deterministic-leaning.
	PurposeClassification Purpose = "classification"

	// PurposeSummarize — condensing inbound content into briefings, ticket
	// summaries. Cost-sensitive at high volume.
	PurposeSummarize Purpose = "summarize"

	// PurposeReason — the agent's main per-turn LLM call inside its
	// tool-use loop. Quality-sensitive; this is where most spend lands.
	PurposeReason Purpose = "reason"

	// PurposeSynthesize — composer-style merging of multiple agent drafts
	// into a single customer-facing reply.
	PurposeSynthesize Purpose = "synthesize"

	// PurposeRedactPII — pre-LLM PII scrubbing pass. Latency-sensitive.
	PurposeRedactPII Purpose = "redact_pii"

	// PurposeBrief — generating the structured briefing handed to agents
	// at stage 7 of the inbound pipeline.
	PurposeBrief Purpose = "brief"

	// PurposeExtractParameters — turning unstructured text into typed
	// arguments for tool calls (parameter extraction).
	PurposeExtractParameters Purpose = "extract_parameters"

	// PurposeRewrite — reply-box rewrite actions such as make-polite,
	// make-shorter, and similar user-triggered composer transformations.
	PurposeRewrite Purpose = "rewrite"

	// PurposeEmbed — vector embedding generation (Voyage). Does NOT fit the
	// LLM preset model (no temperature, max_tokens, or chat shape); see
	// IsNonLLMPurpose.
	PurposeEmbed Purpose = "embed"
)

// KnownPurposes is the canonical set of purposes the platform recognizes.
// The preset registry uses this to validate config/ai_presets.json at
// startup, and the agent handler uses it to validate preset_overrides
// keys at agent save time.
var KnownPurposes = []Purpose{
	PurposeClassification,
	PurposeSummarize,
	PurposeReason,
	PurposeSynthesize,
	PurposeRedactPII,
	PurposeBrief,
	PurposeExtractParameters,
	PurposeRewrite,
	PurposeEmbed,
}

// nonLLMPurposes are purposes that don't fit the LLM preset model. The
// registry skips its "every known purpose has a mapping" check for these,
// but they stay in KnownPurposes so ai_usage.purpose remains type-safe.
var nonLLMPurposes = map[Purpose]struct{}{
	PurposeEmbed: {},
}

// IsKnownPurpose reports whether s names one of the platform's purposes.
func IsKnownPurpose(s string) bool {
	for _, p := range KnownPurposes {
		if string(p) == s {
			return true
		}
	}
	return false
}

// IsNonLLMPurpose reports whether p doesn't resolve to an LLM preset
// (embedding, etc.) and is thus exempt from the purpose_mappings requirement.
func IsNonLLMPurpose(p Purpose) bool {
	_, ok := nonLLMPurposes[p]
	return ok
}
