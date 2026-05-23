package detection

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	cmodels "github.com/abhinavxd/libredesk/internal/conversation/models"
)

// Detection mode constants.
const (
	ModeRulesOnly  = "rules_only"
	ModeAIOnly     = "ai_only"
	ModeRulesAndAI = "rules_and_ai"
	ModeRulesOrAI  = "rules_or_ai"
	ModeManualOnly = "manual_only"
)

// Source constants.
const (
	SourceTicket  = "ticket"
	SourceContact = "contact"
	SourceTags    = "tags"
)

// Logical operators used in ConditionTree.Op.
const (
	LogicalAnd = "AND"
	LogicalOr  = "OR"
	LogicalNot = "NOT"
)

// Comparison operators used in Condition.Operator.
const (
	OpEquals         = "="
	OpNotEquals      = "!="
	OpGreaterThan    = ">"
	OpGreaterOrEqual = ">="
	OpLessThan       = "<"
	OpLessOrEqual    = "<="
	OpContains       = "contains"
	OpNotContains    = "not contains"
	OpContainsRegex  = "contains RegEx"
	OpIn             = "in"
	OpNotIn          = "not in"
	OpIsNull         = "is null"
	OpIsNotNull      = "is not null"
)

// Detection is the top-level config attached to any feature using detection.
// Stored as JSONB on the consuming entity.
type Detection struct {
	Mode  string         `json:"mode"`
	Rules *ConditionTree `json:"rules,omitempty"`
	AI    *AIClassifier  `json:"ai,omitempty"`
}

// ConditionTree is a recursive AND/OR/NOT tree of conditions.
type ConditionTree struct {
	Op       string          `json:"op"` // AND | OR | NOT
	Children []ConditionNode `json:"children"`
}

// ConditionNode is either a subtree or a leaf condition.
// Exactly one of Tree or Leaf should be non-nil.
type ConditionNode struct {
	Tree *ConditionTree `json:"tree,omitempty"`
	Leaf *Condition     `json:"leaf,omitempty"`
}

// Condition is a single comparison at a leaf.
// All string operators are case-insensitive; use OpContainsRegex with
// explicit flags for case-sensitive matching.
type Condition struct {
	Source   string `json:"source"` // ticket | contact | tags
	Path     string `json:"path"`   // dotted path; empty for SourceTags
	Operator string `json:"operator"`
	Value    any    `json:"value"` // typed per operator
}

// AIClassifier is the AI side of a detection.
type AIClassifier struct {
	Prompt string `json:"prompt"`
}

// EvaluationContext is the runtime context passed to Manager.Evaluate.
//
// TraceID is optional: when set by the caller it ties any ai_usage row the
// AI evaluation writes to that trace; left empty, the Manager generates a
// "detection:<uuid>" trace id so the LLM call is still auditable.
type EvaluationContext struct {
	Conversation cmodels.Conversation
	Tags         []string
	TraceID      string
}

// EvaluationResult is the outcome of evaluating a detection.
type EvaluationResult struct {
	Matched   bool    `json:"matched"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// Scan implements sql.Scanner so Detection can be stored directly in JSONB
// columns such as tags.detection.
func (d *Detection) Scan(src any) error {
	if src == nil {
		*d = Detection{Mode: ModeManualOnly}
		return nil
	}

	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("detection: cannot scan %T", src)
	}
	if len(b) == 0 {
		*d = Detection{Mode: ModeManualOnly}
		return nil
	}
	return json.Unmarshal(b, d)
}

// Value implements driver.Valuer for JSONB persistence.
func (d Detection) Value() (driver.Value, error) {
	if d.Mode == "" {
		d.Mode = ModeManualOnly
	}
	return json.Marshal(d)
}
