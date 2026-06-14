package composer

import (
	"testing"

	"github.com/aziz-tb/merio/internal/composer/models"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/stretchr/testify/require"
)

// ticketLeaf / tagsIn build single-leaf condition trees for the tests.
func ticketLeaf(path, op string, value any) *detection.ConditionTree {
	return &detection.ConditionTree{
		Op: detection.LogicalAnd,
		Children: []detection.ConditionNode{
			{Leaf: &detection.Condition{Source: detection.SourceTicket, Path: path, Operator: op, Value: value}},
		},
	}
}

func tagsIn(slug string) detection.ConditionTree {
	return detection.ConditionTree{
		Op: detection.LogicalAnd,
		Children: []detection.ConditionNode{
			{Leaf: &detection.Condition{Source: detection.SourceTags, Operator: detection.OpIn, Value: []string{slug}}},
		},
	}
}

func guidanceIDs(items []models.GuidanceItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

// TestAssemble_GuidanceMatching proves conditions SCOPE guidance, and — the key
// Slice-5 finding — that the stored verification level is referenceable today as
// ticket.verification_level with no engine change.
func TestAssemble_GuidanceMatching(t *testing.T) {
	eff := Effective{
		Guidance: []models.GuidanceItem{
			{ID: "always", BodyText: "Use a warm tone.", Status: models.StatusLive}, // nil conditions → always
			{ID: "unverified", BodyText: "Ask for verification before account actions.", Status: models.StatusLive,
				Conditions: ticketLeaf("verification_level", detection.OpLessThan, 2)},
			{ID: "verified", BodyText: "Offer the self-service portal.", Status: models.StatusLive,
				Conditions: ticketLeaf("verification_level", detection.OpGreaterOrEqual, 2)},
		},
	}

	// Unverified ticket (level 0): "always" + "unverified" apply.
	unver := Assemble(eff, detection.EvaluationContext{Conversation: cmodels.Conversation{VerificationLevel: 0}})
	require.ElementsMatch(t, []string{"always", "unverified"}, guidanceIDs(unver.Guidance))

	// Verified ticket (level 2): "always" + "verified" apply.
	ver := Assemble(eff, detection.EvaluationContext{Conversation: cmodels.Conversation{VerificationLevel: 2}})
	require.ElementsMatch(t, []string{"always", "verified"}, guidanceIDs(ver.Guidance))
}

// TestAssemble_EscalationRulesGate proves rules GATE deterministically: a tag
// rule fires only when the tag is present, and Escalate() reflects it.
func TestAssemble_EscalationRulesGate(t *testing.T) {
	eff := Effective{EscalationRules: []detection.ConditionTree{tagsIn("legal_complaint")}}

	fired := Assemble(eff, detection.EvaluationContext{Tags: []string{"legal_complaint", "urgent"}})
	require.Len(t, fired.FiredEscalationRules, 1)
	esc, reason := fired.Escalate()
	require.True(t, esc)
	require.NotEmpty(t, reason)

	absent := Assemble(eff, detection.EvaluationContext{Tags: []string{"billing"}})
	require.Empty(t, absent.FiredEscalationRules)
	esc2, _ := absent.Escalate()
	require.False(t, esc2)
}

// TestAssemble_EmptyConditionAsymmetry pins the deliberate split: empty guidance
// conditions are always relevant (permissive); an empty escalation rule never
// fires (conservative).
func TestAssemble_EmptyConditionAsymmetry(t *testing.T) {
	emptyTree := &detection.ConditionTree{Op: detection.LogicalAnd} // no children
	eff := Effective{
		Guidance:        []models.GuidanceItem{{ID: "g", BodyText: "x", Status: models.StatusLive, Conditions: emptyTree}},
		EscalationRules: []detection.ConditionTree{{Op: detection.LogicalAnd}},
	}
	got := Assemble(eff, detection.EvaluationContext{})
	require.Len(t, got.Guidance, 1, "empty guidance conditions → always relevant")
	require.Empty(t, got.FiredEscalationRules, "empty escalation rule → never fires")
}

// TestAssembled_GuidanceProse proves matching prose (ordinary + escalation
// guidance) renders into one block, blanks are dropped, and no guidance → "".
func TestAssembled_GuidanceProse(t *testing.T) {
	a := Assembled{
		Guidance:           []models.GuidanceItem{{BodyText: "Use Sie."}, {BodyText: "   "}},
		EscalationGuidance: []models.GuidanceItem{{BodyText: "Escalate legal threats."}},
	}
	prose := a.GuidanceProse()
	require.Contains(t, prose, "Use Sie.")
	require.Contains(t, prose, "Escalate legal threats.")
	require.NotContains(t, prose, "-  ") // the blank item produced no bullet

	require.Empty(t, Assembled{}.GuidanceProse(), "no guidance → empty so the caller appends nothing")
}
