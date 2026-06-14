package composer

import (
	"testing"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/stretchr/testify/require"
)

const (
	inboxA = 1
	inboxB = 2
)

func ids(items []models.GuidanceItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// sampleConfig: guidance + escalation items scoped to different inboxes (some
// unscoped, one paused), an automation block, and one global escalation rule.
func sampleConfig() models.Config {
	live := models.StatusLive
	paused := models.StatusPaused
	g := models.GroupCompanyContext
	return models.Config{
		Guidance: []models.GuidanceItem{
			{ID: "g-A", Group: g, Status: live, InboxScope: []int{inboxA}},          // A only
			{ID: "g-B", Group: g, Status: live, InboxScope: []int{inboxB}},          // B only
			{ID: "g-all", Group: g, Status: live, InboxScope: nil},                  // all inboxes
			{ID: "g-A-paused", Group: g, Status: paused, InboxScope: []int{inboxA}}, // excluded (paused)
		},
		Automation: models.Automation{
			AutoSend:                  true,
			ApprovalRequiredToolSlugs: []string{"cancel_contract"},
		},
		Escalation: models.Escalation{
			Rules: []detection.ConditionTree{
				{Op: detection.LogicalAnd, Children: []detection.ConditionNode{
					{Leaf: &detection.Condition{Source: detection.SourceTags, Operator: detection.OpIn, Value: "legal"}},
				}},
			},
			Guidance: []models.GuidanceItem{
				{ID: "e-A", Group: g, Status: live, InboxScope: []int{inboxA}},
				{ID: "e-all", Group: g, Status: live, InboxScope: nil},
				{ID: "e-B-paused", Group: g, Status: paused, InboxScope: []int{inboxB}},
			},
		},
	}
}

func TestResolve_InboxScopeAndStatus(t *testing.T) {
	cfg := sampleConfig()

	effA := Resolve(cfg, inboxA)
	require.ElementsMatch(t, []string{"g-A", "g-all"}, ids(effA.Guidance),
		"inbox A gets its scoped item + unscoped, paused excluded")
	require.ElementsMatch(t, []string{"e-A", "e-all"}, ids(effA.EscalationGuidance))

	effB := Resolve(cfg, inboxB)
	require.ElementsMatch(t, []string{"g-B", "g-all"}, ids(effB.Guidance),
		"inbox B gets its scoped item + unscoped")
	require.ElementsMatch(t, []string{"e-all"}, ids(effB.EscalationGuidance),
		"e-A is A-scoped; e-B-paused is paused")

	// Escalation rules are global (no per-rule scope) — present for every inbox.
	require.Len(t, effA.EscalationRules, 1)
	require.Len(t, effB.EscalationRules, 1)

	// Automation passes through unchanged.
	require.True(t, effA.Automation.AutoSend)
	require.Equal(t, []string{"cancel_contract"}, effA.Automation.ApprovalRequiredToolSlugs)
}

func TestResolve_EmptyConfig(t *testing.T) {
	eff := Resolve(models.Config{}, inboxA)
	require.Empty(t, eff.Guidance)
	require.Empty(t, eff.EscalationRules)
	require.Empty(t, eff.EscalationGuidance)
	require.False(t, eff.Automation.AutoSend)
}

// TestResolve_NoConditionEvaluation documents that the resolver filters by
// inbox scope + status ONLY: an item with conditions that would NOT match a
// ticket is still returned (condition evaluation is Slice 5, against a live
// ticket).
func TestResolve_NoConditionEvaluation(t *testing.T) {
	cfg := models.Config{Guidance: []models.GuidanceItem{{
		ID: "g-cond", Group: models.GroupOther, Status: models.StatusLive,
		Conditions: &detection.ConditionTree{Op: detection.LogicalAnd, Children: []detection.ConditionNode{
			{Leaf: &detection.Condition{Source: detection.SourceTicket, Path: "subject", Operator: detection.OpContains, Value: "nope"}},
		}},
	}}}
	eff := Resolve(cfg, inboxA)
	require.Equal(t, []string{"g-cond"}, ids(eff.Guidance), "conditions are not evaluated at resolve time")
}
