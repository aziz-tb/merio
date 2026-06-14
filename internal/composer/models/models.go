// Package models holds DB-wire and typed domain types for the Composer
// configuration — the single global config that layers situational behavior
// (guidance, automation, escalation) on top of the platform-owned Composer
// persona. There is ONE global config row; per-inbox differences are expressed
// as inbox scope on individual items (Model A — AI_LAYER_DESIGN.md §7), not as
// separate per-inbox config objects.
//
// Pattern A: the three sections are stored as raw JSONB on ComposerConfig and
// decoded into the typed Config at the handler/runtime boundary (Decode), never
// in the model layer.
package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aziz-tb/merio/internal/detection"
)

// Guidance item groups. A guidance item belongs to exactly one.
const (
	GroupCompanyContext     = "company_context"
	GroupCommunicationStyle = "communication_style"
	GroupSpam               = "spam"
	GroupOther              = "other"
)

// Guidance item statuses. Paused items are excluded by the resolver.
const (
	StatusLive   = "live"
	StatusPaused = "paused"
)

// GuidanceItem is one prose instruction the Composer may apply, scoped by inbox
// and optionally gated for relevance by conditions. Conditions SCOPE (decide
// relevance) — they do not gate; the Composer still reasons about whether/how
// to apply the item. Empty InboxScope = all inboxes; nil Conditions = always
// relevant. Conditions reuse the detection AST.
type GuidanceItem struct {
	ID         string                   `json:"id"`
	Title      string                   `json:"title"`
	Group      string                   `json:"group"`
	BodyText   string                   `json:"body_text"`
	InboxScope []int                    `json:"inbox_scope"`
	Conditions *detection.ConditionTree `json:"conditions,omitempty"`
	Status     string                   `json:"status"`
}

// Automation holds the Composer's send + approval switches. AutoSend is global
// (not inbox-scoped in v1). ApprovalRequiredToolSlugs is the single source the
// Slice 4 approval gate reads: a tool pauses for approval iff its slug is in
// this list, at the dispatch point and origin-agnostic (Composer or sub-agent).
type Automation struct {
	AutoSend                  bool     `json:"auto_send"`
	ApprovalRequiredToolSlugs []string `json:"approval_required_tool_slugs"`
}

// Escalation holds deterministic escalation Rules (condition trees that force
// escalation when matched — evaluated in Slice 5) and escalation Guidance
// (prose the Composer may follow, same shape as a guidance item). Rules carry
// no per-rule inbox scope: an inbox-specific rule is expressed as a condition
// leaf, or as scoped escalation guidance.
type Escalation struct {
	Rules    []detection.ConditionTree `json:"rules"`
	Guidance []GuidanceItem            `json:"guidance"`
}

// Config is the decoded, typed Composer configuration. The resolver and the
// validator operate on this, never on the raw JSONB.
type Config struct {
	Guidance   []GuidanceItem `json:"guidance"`
	Automation Automation     `json:"automation"`
	Escalation Escalation     `json:"escalation"`
}

// ComposerConfig is the DB-wire row for the singleton composer_config table
// (id is always 1). The three sections are raw JSONB here; decode to Config via
// Decode at the boundary.
type ComposerConfig struct {
	ID         int             `db:"id" json:"-"`
	Guidance   json.RawMessage `db:"guidance" json:"guidance"`
	Automation json.RawMessage `db:"automation" json:"automation"`
	Escalation json.RawMessage `db:"escalation" json:"escalation"`
	CreatedAt  time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updated_at"`
}

// Decode unmarshals the raw JSONB sections into the typed Config. Empty columns
// decode to zero-value (empty) sections, so a freshly-seeded or absent config
// yields an empty-but-valid Config the resolver can run over.
func (c ComposerConfig) Decode() (Config, error) {
	cfg := Config{}
	if len(c.Guidance) > 0 {
		if err := json.Unmarshal(c.Guidance, &cfg.Guidance); err != nil {
			return Config{}, fmt.Errorf("composer: decode guidance: %w", err)
		}
	}
	if len(c.Automation) > 0 {
		if err := json.Unmarshal(c.Automation, &cfg.Automation); err != nil {
			return Config{}, fmt.Errorf("composer: decode automation: %w", err)
		}
	}
	if len(c.Escalation) > 0 {
		if err := json.Unmarshal(c.Escalation, &cfg.Escalation); err != nil {
			return Config{}, fmt.Errorf("composer: decode escalation: %w", err)
		}
	}
	return cfg, nil
}
