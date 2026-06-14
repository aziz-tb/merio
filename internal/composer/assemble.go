package composer

import (
	"fmt"
	"strings"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
)

// Assembled is the run-relevant Composer behavior for ONE ticket: the guidance
// whose conditions match (or are unconditional), the escalation guidance that
// matches, the escalation rules that actually FIRED, and the automation
// settings. It is produced by Assemble from an Effective (already filtered to
// live + in-scope by Resolve) plus a live ticket EvaluationContext.
//
// Assemble is PURE and shareable on purpose: the Composer run loop and a future
// ticket-prompt preview both build the same Assembled the same way, so what the
// operator previews is exactly what the run sees. No DB, no run-loop coupling —
// the caller hydrates the EvaluationContext and passes it in.
type Assembled struct {
	Guidance             []models.GuidanceItem
	EscalationGuidance   []models.GuidanceItem
	FiredEscalationRules []detection.ConditionTree
	Automation           models.Automation
	Capability           models.Capability
}

// Assemble evaluates each live, in-scope item's conditions against the ticket
// context and returns what actually applies.
//
// The asymmetry is deliberate and matches AI_LAYER_DESIGN.md §7:
//   - Guidance conditions SCOPE (relevance). Empty/absent conditions = always
//     relevant — the permissive default, since the Composer still reasons about
//     whether to apply the prose.
//   - Escalation rules GATE (deterministic). An empty rule never fires — the
//     conservative default, since a firing rule forces escalation regardless of
//     model judgment.
//
// Reuses the detection engine's exported rules evaluator; never re-implements
// condition evaluation.
func Assemble(eff Effective, ec detection.EvaluationContext) Assembled {
	return Assembled{
		Guidance:             matchingGuidance(eff.Guidance, ec),
		EscalationGuidance:   matchingGuidance(eff.EscalationGuidance, ec),
		FiredEscalationRules: firedRules(eff.EscalationRules, ec),
		Automation:           eff.Automation,
		Capability:           eff.Capability, // global; not condition-evaluated
	}
}

// matchingGuidance keeps the items whose conditions are satisfied — or that have
// no conditions (always relevant).
func matchingGuidance(items []models.GuidanceItem, ec detection.EvaluationContext) []models.GuidanceItem {
	out := make([]models.GuidanceItem, 0, len(items))
	for _, it := range items {
		if unconditional(it.Conditions) || detection.EvaluateRules(it.Conditions, ec, nil) {
			out = append(out, it)
		}
	}
	return out
}

// firedRules keeps the escalation rules whose condition tree matched. An empty
// rule evaluates false (EvaluateRules returns false for a childless tree), so it
// never fires — the conservative gate default.
func firedRules(rules []detection.ConditionTree, ec detection.EvaluationContext) []detection.ConditionTree {
	out := make([]detection.ConditionTree, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		if detection.EvaluateRules(&rule, ec, nil) {
			out = append(out, rule)
		}
	}
	return out
}

// unconditional reports whether a guidance item has no conditions to evaluate —
// nil, or an empty tree — which means "always relevant" (§7).
func unconditional(c *detection.ConditionTree) bool {
	return c == nil || len(c.Children) == 0
}

// GuidanceProse renders the matching guidance (ordinary + escalation guidance)
// into one labelled block for the Composer's system prompt. Escalation guidance
// is folded in as ordinary prose — the model decides whether to act on it; the
// deterministic escalation arm is FiredEscalationRules, not this. Returns "" when
// nothing matches, so the caller appends nothing.
func (a Assembled) GuidanceProse() string {
	var b strings.Builder
	appendItems := func(items []models.GuidanceItem) {
		for _, it := range items {
			body := strings.TrimSpace(it.BodyText)
			if body == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString("- ")
			b.WriteString(body)
		}
	}
	appendItems(a.Guidance)
	appendItems(a.EscalationGuidance)
	if b.Len() == 0 {
		return ""
	}
	return "Operator guidance for this conversation (apply where relevant):\n" + b.String()
}

// Escalate reports whether a deterministic escalation rule fired — the hard arm
// of the escalation union (§4) — and an internal reason for the escalation
// record. The model-judgment arms (the escalate tool, escalation guidance) are
// separate; this is the platform escalating regardless of model output.
func (a Assembled) Escalate() (bool, string) {
	if len(a.FiredEscalationRules) == 0 {
		return false, ""
	}
	return true, fmt.Sprintf("Escalated by a deterministic escalation rule (%d matched).", len(a.FiredEscalationRules))
}
