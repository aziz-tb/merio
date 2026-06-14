package composer

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
)

// validGroups / validStatuses are the allowed enum sets for a guidance item.
var validGroups = map[string]bool{
	models.GroupCompanyContext:     true,
	models.GroupCommunicationStyle: true,
	models.GroupSpam:               true,
	models.GroupOther:              true,
}

var validStatuses = map[string]bool{
	models.StatusLive:   true,
	models.StatusPaused: true,
}

// ValidationRefs carries the reference data ValidateConfig checks against,
// gathered by the handler from the managers so the validator stays pure and
// DB-free (unit-testable). WriteToolSlugs is the set of existing custom WRITE
// tool slugs; InboxIDs the set of existing inbox ids; AllowedSources the
// detection sources a condition leaf may reference.
type ValidationRefs struct {
	WriteToolSlugs map[string]bool
	InboxIDs       map[int]bool
	AllowedSources []string
}

// DecodeStrict decodes an operator config payload, rejecting unknown fields at
// every level (known-keys + type check, the first stage of Pattern-A
// validation). The nested detection AST decodes cleanly because its own fields
// are all known to the decoder.
func DecodeStrict(raw []byte) (models.Config, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg models.Config
	if err := dec.Decode(&cfg); err != nil {
		return models.Config{}, err
	}
	return cfg, nil
}

// ValidateConfig enforces the semantic + reference rules (group/status enums,
// inbox-scope existence, approval-slug existence, condition source/operator
// validity). Runs after DecodeStrict has handled known-keys/type. Pure: the
// handler supplies refs.
func ValidateConfig(cfg models.Config, refs ValidationRefs) error {
	for i := range cfg.Guidance {
		if err := validateGuidanceItem(cfg.Guidance[i], refs, fmt.Sprintf("guidance[%d]", i)); err != nil {
			return err
		}
	}
	for _, slug := range cfg.Automation.ApprovalRequiredToolSlugs {
		if !refs.WriteToolSlugs[slug] {
			return fmt.Errorf("automation.approval_required_tool_slugs references %q, which is not an existing custom write tool", slug)
		}
	}
	for i := range cfg.Escalation.Rules {
		if err := detection.ValidateConditionTree(&cfg.Escalation.Rules[i], refs.AllowedSources); err != nil {
			return fmt.Errorf("escalation.rules[%d]: %w", i, err)
		}
	}
	for i := range cfg.Escalation.Guidance {
		if err := validateGuidanceItem(cfg.Escalation.Guidance[i], refs, fmt.Sprintf("escalation.guidance[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateGuidanceItem(item models.GuidanceItem, refs ValidationRefs, ctx string) error {
	if !validGroups[item.Group] {
		return fmt.Errorf("%s.group %q is invalid", ctx, item.Group)
	}
	if !validStatuses[item.Status] {
		return fmt.Errorf("%s.status %q is invalid", ctx, item.Status)
	}
	for _, id := range item.InboxScope {
		if !refs.InboxIDs[id] {
			return fmt.Errorf("%s.inbox_scope references nonexistent inbox id=%d", ctx, id)
		}
	}
	if item.Conditions != nil {
		if err := detection.ValidateConditionTree(item.Conditions, refs.AllowedSources); err != nil {
			return fmt.Errorf("%s.conditions: %w", ctx, err)
		}
	}
	return nil
}
