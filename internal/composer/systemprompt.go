package composer

import (
	"sort"
	"strings"

	"github.com/aziz-tb/merio/internal/composer/models"
)

// BuildComposerSystemPrompt assembles the Composer's full system prompt as a
// single production-grade briefing, in reasoning order. It is PURE: the run path
// (via the activity) and the future ticket-prompt preview build it identically
// from the same inputs, so the preview shows exactly what the run sees.
//
// skeleton and decisionInstructions are platform-owned constants (runner-owned,
// passed in so this package stays decoupled from the runner). roster is the
// pre-formatted body of the available-specialists list (slug — description
// lines, no header), built from the live enabled sub-agents. a is the
// condition-evaluated guidance.
//
// Sections, each OMITTED ENTIRELY when empty, with instructive headers:
//  1. Identity / role (skeleton)
//  2. Company context            (guidance group: company_context)
//  3. How to communicate         (guidance group: communication_style)
//  4. Available specialists      (roster)
//  5. Situational guidance        (guidance group: other)
//  6. Identifying and handling spam (guidance group: spam)
//  7. Decision & escalation (LAST): decisionInstructions + escalation guidance,
//     co-located so "when to escalate" sits with the escalate contract.
//
// Stable→variable: the skeleton + always-on groups precede the roster (changes
// only when operators add/remove agents) and the condition-matched/ticket-
// varying content, so a future cache breakpoint can sit after the stable prefix.
// Deterministic: sections are fixed-order and items are sorted by id (never map
// iteration), so identical inputs yield a byte-identical prompt. With empty
// guidance + empty roster this reduces to exactly "skeleton\n\ndecisionInstructions"
// — byte-identical to the runner's minimal fallback floor (see the runner's
// MinimalComposerSystemPrompt + the airuns drift-guard test).
func BuildComposerSystemPrompt(skeleton, decisionInstructions, roster string, a Assembled) string {
	sections := make([]string, 0, 7)
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			sections = append(sections, s)
		}
	}

	add(skeleton)
	add(groupSection("Company context", a.Guidance, models.GroupCompanyContext))
	add(groupSection("How to communicate", a.Guidance, models.GroupCommunicationStyle))
	add(rosterSection(roster))
	add(groupSection("Situational guidance", a.Guidance, models.GroupOther))
	add(groupSection("Identifying and handling spam", a.Guidance, models.GroupSpam))
	add(decisionSection(decisionInstructions, a.EscalationGuidance))

	return strings.Join(sections, "\n\n")
}

// rosterSection wraps the pre-formatted roster body with its header, or "" when
// no specialists exist (the section is omitted entirely).
func rosterSection(roster string) string {
	if strings.TrimSpace(roster) == "" {
		return ""
	}
	return "## Available specialists\n" + strings.TrimSpace(roster)
}

// decisionSection places the terminal-decision contract last, with any
// escalation guidance co-located under it (so "when to escalate" sits next to
// the escalate instruction, not folded into general guidance up top).
func decisionSection(decisionInstructions string, escalation []models.GuidanceItem) string {
	parts := make([]string, 0, 2)
	if d := strings.TrimSpace(decisionInstructions); d != "" {
		parts = append(parts, d)
	}
	if bodies := bodies(escalation); len(bodies) > 0 {
		parts = append(parts, "## When to escalate\n"+strings.Join(bodies, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// groupSection renders the matching guidance items of one group under an
// instructive header, or "" when the group has no items (header omitted too).
func groupSection(header string, items []models.GuidanceItem, group string) string {
	inGroup := make([]models.GuidanceItem, 0, len(items))
	for _, it := range items {
		if it.Group == group {
			inGroup = append(inGroup, it)
		}
	}
	lines := bodies(inGroup)
	if len(lines) == 0 {
		return ""
	}
	return "## " + header + "\n" + strings.Join(lines, "\n")
}

// bodies returns the non-empty body texts of items as "- " bullets, sorted by id
// for deterministic (byte-stable) output.
func bodies(items []models.GuidanceItem) []string {
	sorted := make([]models.GuidanceItem, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.BodyText) != "" {
			sorted = append(sorted, it)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := make([]string, 0, len(sorted))
	for _, it := range sorted {
		out = append(out, "- "+strings.TrimSpace(it.BodyText))
	}
	return out
}
