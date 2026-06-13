// Package models holds DB-wire and domain types for the ai_agents table.
// The detection JSONB column reuses the universal detection AST so one
// condition language is maintained platform-wide.
package models

import (
	"encoding/json"
	"time"
)

// Agent is the DB-wire representation of an ai_agents row. The JSONB columns
// are decoded at the handler/runtime layer into:
//
//   - Detection: internal/detection.Detection. New agents get the
//     DefaultDetection scaffold from internal/ai/agents so the visual
//     builder has usable data.
//   - Tools: []ToolRef
//   - PresetOverrides: map[string]string (purpose → preset slug)
//   - Autonomy: Autonomy
//
// Description allows NULL in the DB but handlers always pass a non-NULL string.
type Agent struct {
	ID                      int             `db:"id" json:"id"`
	CreatedAt               time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time       `db:"updated_at" json:"updated_at"`
	Name                    string          `db:"name" json:"name"`
	Slug                    string          `db:"slug" json:"slug"`
	Description             string          `db:"description" json:"description"`
	Prompt                  string          `db:"prompt" json:"prompt"`
	Detection               json.RawMessage `db:"detection" json:"detection"`
	Tools                   json.RawMessage `db:"tools" json:"tools"`
	KBScope                 json.RawMessage `db:"kb_scope" json:"kb_scope"`
	PresetOverrides         json.RawMessage `db:"preset_overrides" json:"preset_overrides"`
	ConfidenceThreshold     float64         `db:"confidence_threshold" json:"confidence_threshold"`
	MaxIterations           int             `db:"max_iterations" json:"max_iterations"`
	ExtendedThinkingEnabled bool            `db:"extended_thinking_enabled" json:"extended_thinking_enabled"`
	MinVerification         int             `db:"min_verification" json:"min_verification"`
	Autonomy                json.RawMessage `db:"autonomy" json:"autonomy"`
	Enabled                 bool            `db:"enabled" json:"enabled"`
}

// KBScope is the structured shape of the `kb_scope` JSONB column. FolderIDs
// whitelists kb_folders.id; ArticleIDs additively whitelists kb_articles.id
// outside those folders. Empty scope = no KB access: kb_search returns empty
// without consulting the index.
type KBScope struct {
	FolderIDs  []int `json:"folder_ids"`
	ArticleIDs []int `json:"article_ids"`
}

// ToolRefKind distinguishes internal capabilities (code under
// internal/ai/tools/builtin/) from custom tools (ai_tools rows).
type ToolRefKind string

// The recognized ToolRefKind values.
const (
	ToolRefKindInternal ToolRefKind = "internal"
	ToolRefKindCustom   ToolRefKind = "custom"
)

// ToolRef is one entry in the agent's `tools` JSONB array. ID is raw JSON to
// carry both numeric ids (custom, ai_tools.id) and string ids (internal
// capability IDs). Use the CustomID()/InternalID() accessors rather than
// reading ID directly.
type ToolRef struct {
	Kind           ToolRefKind     `json:"kind"`
	ID             json.RawMessage `json:"id"`
	ConfigOverride json.RawMessage `json:"config_override,omitempty"`
}

// CustomID returns the ai_tools.id for a custom-kind ref, or (0, false)
// if Kind is not Custom or ID isn't a JSON number.
func (r ToolRef) CustomID() (int, bool) {
	if r.Kind != ToolRefKindCustom {
		return 0, false
	}
	var id int
	if err := json.Unmarshal(r.ID, &id); err != nil {
		return 0, false
	}
	return id, true
}

// InternalID returns the internal capability id for an internal-kind
// ref, or ("", false) if Kind is not Internal or ID isn't a JSON string.
func (r ToolRef) InternalID() (string, bool) {
	if r.Kind != ToolRefKindInternal {
		return "", false
	}
	var id string
	if err := json.Unmarshal(r.ID, &id); err != nil {
		return "", false
	}
	return id, true
}

// Autonomy is the structured shape of the `autonomy` JSONB — per-agent GATE
// policy thresholds. Zero values mean "use the system default for the preset."
type Autonomy struct {
	MinConfidenceSend      float64 `json:"min_confidence_send"`
	MinConfidenceDraft     float64 `json:"min_confidence_draft"`
	EscalateOnDisagreement bool    `json:"escalate_on_disagreement"`
}
