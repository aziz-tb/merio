package composer

import (
	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
)

// Effective is the resolved set of Composer behavior that applies to one inbox:
// the live guidance items, escalation rules, escalation guidance, and automation
// settings in scope for that inbox.
type Effective struct {
	Guidance           []models.GuidanceItem
	Automation         models.Automation
	EscalationRules    []detection.ConditionTree
	EscalationGuidance []models.GuidanceItem
}

// Resolve filters a Config down to what applies to inboxID. It filters by inbox
// scope and live/paused status ONLY — it does NOT evaluate the guidance or
// escalation AST conditions against a ticket (that is Slice 5, against a live
// ticket). Pure and side-effect-free: no DB, no run-loop coupling.
//
// Slice 5 (assemble effective guidance into the Composer's prompt, evaluate
// conditions) and a future ticket-prompt preview both reuse this resolver —
// keep it pure and callable from more than the run loop; never duplicate the
// scoping logic.
func Resolve(cfg models.Config, inboxID int) Effective {
	return Effective{
		Guidance:           filterLiveScoped(cfg.Guidance, inboxID),
		Automation:         cfg.Automation,
		EscalationRules:    cfg.Escalation.Rules, // deterministic + global; no per-rule scope
		EscalationGuidance: filterLiveScoped(cfg.Escalation.Guidance, inboxID),
	}
}

// filterLiveScoped keeps items that are live AND in scope for inboxID.
func filterLiveScoped(items []models.GuidanceItem, inboxID int) []models.GuidanceItem {
	out := make([]models.GuidanceItem, 0, len(items))
	for _, it := range items {
		if it.Status != models.StatusLive {
			continue
		}
		if inboxInScope(inboxID, it.InboxScope) {
			out = append(out, it)
		}
	}
	return out
}

// inboxInScope reports whether inboxID is covered by scope. An empty scope means
// "all inboxes".
func inboxInScope(inboxID int, scope []int) bool {
	if len(scope) == 0 {
		return true
	}
	for _, id := range scope {
		if id == inboxID {
			return true
		}
	}
	return false
}
