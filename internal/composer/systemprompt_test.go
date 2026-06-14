package composer

import (
	"strings"
	"testing"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/stretchr/testify/require"
)

const (
	testSkeleton = "SKELETON: you are the composer."
	testDecision = "DECISION: end with answer or escalate."
)

func gi(id, group, body string) models.GuidanceItem {
	return models.GuidanceItem{ID: id, Group: group, BodyText: body, Status: models.StatusLive}
}

// TestBuildComposerSystemPrompt_Structure proves the seven-section reasoning
// order, instructive headers, roster placement, and escalation-in-decision.
func TestBuildComposerSystemPrompt_Structure(t *testing.T) {
	a := Assembled{
		Guidance: []models.GuidanceItem{
			gi("c1", models.GroupCompanyContext, "We are Acme."),
			gi("s1", models.GroupCommunicationStyle, "Use Sie."),
			gi("o1", models.GroupOther, "Offer the portal."),
			gi("sp1", models.GroupSpam, "Ignore crypto spam."),
		},
		EscalationGuidance: []models.GuidanceItem{gi("e1", "", "Escalate legal threats.")},
	}
	out := BuildComposerSystemPrompt(testSkeleton, testDecision, "- billing — invoices", a)

	for _, want := range []string{
		testSkeleton,
		"## Company context", "We are Acme.",
		"## How to communicate", "Use Sie.",
		"## Available specialists", "- billing — invoices",
		"## Situational guidance", "Offer the portal.",
		"## Identifying and handling spam", "Ignore crypto spam.",
		testDecision, "## When to escalate", "Escalate legal threats.",
	} {
		require.Contains(t, out, want)
	}

	// Reasoning order: skeleton < company < comm < roster < situational < spam < decision.
	last := -1
	for _, marker := range []string{
		testSkeleton, "## Company context", "## How to communicate",
		"## Available specialists", "## Situational guidance",
		"## Identifying and handling spam", testDecision,
	} {
		idx := strings.Index(out, marker)
		require.Greater(t, idx, last, "section %q out of reasoning order", marker)
		last = idx
	}

	// Escalation guidance is co-located in the DECISION section, not up top.
	require.Greater(t, strings.Index(out, "## When to escalate"), strings.Index(out, testDecision))
	require.Greater(t, strings.Index(out, "## When to escalate"), strings.Index(out, "## Situational guidance"))
}

// TestBuildComposerSystemPrompt_EmptyOmitted proves empty sections (incl. an
// empty roster) produce no header, and the degenerate input is exactly the floor
// — the same string the runner's MinimalComposerSystemPrompt yields.
func TestBuildComposerSystemPrompt_EmptyOmitted(t *testing.T) {
	out := BuildComposerSystemPrompt(testSkeleton, testDecision, "", Assembled{})
	require.Equal(t, testSkeleton+"\n\n"+testDecision, out)
	require.NotContains(t, out, "##", "no section headers when everything is empty")
}

// TestBuildComposerSystemPrompt_SkeletonAlone: the skeleton with nothing else is
// just the skeleton (complete + coherent on its own — guidance is additive).
func TestBuildComposerSystemPrompt_SkeletonAlone(t *testing.T) {
	require.Equal(t, testSkeleton, BuildComposerSystemPrompt(testSkeleton, "", "", Assembled{}))
}

// TestBuildComposerSystemPrompt_Deterministic: identical inputs → byte-identical
// output, with within-group items sorted by id (not insertion/map order).
func TestBuildComposerSystemPrompt_Deterministic(t *testing.T) {
	a1 := Assembled{Guidance: []models.GuidanceItem{gi("b", models.GroupOther, "B"), gi("a", models.GroupOther, "A")}}
	a2 := Assembled{Guidance: []models.GuidanceItem{gi("a", models.GroupOther, "A"), gi("b", models.GroupOther, "B")}}
	out1 := BuildComposerSystemPrompt(testSkeleton, testDecision, "", a1)
	require.Equal(t, out1, BuildComposerSystemPrompt(testSkeleton, testDecision, "", a2))
	require.Less(t, strings.Index(out1, "- A"), strings.Index(out1, "- B"), "items sorted by id")
}

// TestBuildComposerSystemPrompt_StableBeforeVariable: always-on company/comm
// guidance precedes the roster and the (typically ticket-varying) situational
// section, keeping the longest stable prefix first.
func TestBuildComposerSystemPrompt_StableBeforeVariable(t *testing.T) {
	a := Assembled{Guidance: []models.GuidanceItem{
		gi("c", models.GroupCompanyContext, "stable company"),
		gi("o", models.GroupOther, "variable situational"),
	}}
	out := BuildComposerSystemPrompt(testSkeleton, testDecision, "- billing — x", a)
	require.Less(t, strings.Index(out, "stable company"), strings.Index(out, "- billing — x"))
	require.Less(t, strings.Index(out, "- billing — x"), strings.Index(out, "variable situational"))
}
