package airuns

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/composer"
	composermodels "github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/contract"
	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/customer"
	"github.com/aziz-tb/merio/internal/detection"
)

// evaluationContextFor hydrates the detection EvaluationContext for a live
// conversation — the ticket plus its customer and contract — so Composer
// guidance/escalation conditions evaluate against current reality.
//
// This mirrors the hydration in internal/activities/agentvalidation and
// internal/automation (this is the third such site). It is kept INLINE per
// KISS: those two callers use different manager abstractions (automation wraps
// narrow interfaces with their own test doubles; agentvalidation uses the
// managers directly), so unifying all three behind one helper is a clean,
// behavior-preserving follow-up rather than in-scope churn for this slice.
//
// Best-effort: a hydrate failure is logged and leaves that source unset (a
// condition referencing it simply won't match) — it never fails the run.
func (a *Activities) evaluationContextFor(ctx context.Context, conv convmodels.Conversation) detection.EvaluationContext {
	ec := detection.EvaluationContext{
		Conversation: conv,
		Tags:         conv.TagSlugs(),
		TraceID:      conv.UUID,
	}
	if conv.CustomerID.Valid && a.Customers != nil {
		c, err := a.Customers.GetFreshByCustomerID(ctx, conv.CustomerID.String, customer.FreshOptions{})
		if err != nil {
			if a.Lo != nil {
				a.Lo.Warn("composer assemble: hydrate customer failed", "conversation_uuid", conv.UUID, "error", err)
			}
		} else {
			ec.Customer = c
		}
	}
	if conv.ContractID.Valid && a.Contracts != nil {
		c, err := a.Contracts.GetFreshByContractID(ctx, conv.ContractID.String, contract.FreshOptions{})
		if err != nil {
			if a.Lo != nil {
				a.Lo.Warn("composer assemble: hydrate contract failed", "conversation_uuid", conv.UUID, "error", err)
			}
		} else {
			ec.Contract = c
		}
	}
	return ec
}

// assembleFor loads the single global Composer config, resolves it for inboxID
// (Slice 3), and assembles it against the live conversation (Slice 5): the
// guidance whose conditions match, the escalation guidance that matches, the
// escalation rules that fired, and the automation settings.
//
// Best-effort: ok=false when the config can't be loaded/decoded, so the caller
// leaves the run unshaped rather than failing it. Guidance and escalation are
// additive — never run-blocking.
func (a *Activities) assembleFor(ctx context.Context, inboxID int, conv convmodels.Conversation) (composer.Assembled, bool) {
	if a.Composer == nil {
		return composer.Assembled{}, false
	}
	row, err := a.Composer.GetConfig()
	if err != nil {
		if a.Lo != nil {
			a.Lo.Warn("composer assemble: get config failed", "error", err)
		}
		return composer.Assembled{}, false
	}
	cfg, err := row.Decode()
	if err != nil {
		if a.Lo != nil {
			a.Lo.Warn("composer assemble: decode config failed", "error", err)
		}
		return composer.Assembled{}, false
	}
	return composer.Assemble(composer.Resolve(cfg, inboxID), a.evaluationContextFor(ctx, conv)), true
}

// composerRoster formats the live enabled sub-agents into the body of the
// Composer's "Available specialists" section — one "- <slug> — <description>"
// line per agent, sorted by id for deterministic output. It is the FULL catalog
// of enabled specialists, always: detection may later SUGGEST relevance (Slice
// 6, additive) but must never filter this list — the Composer sees every
// specialist and decides who to call. Returns "" when none are enabled (the
// section is omitted). Best-effort: a load failure logs and yields no roster.
func (a *Activities) composerRoster(ctx context.Context) string {
	if a.Agents == nil {
		return ""
	}
	agents, err := a.Agents.ListEnabled(ctx)
	if err != nil {
		if a.Lo != nil {
			a.Lo.Warn("composer roster: list enabled agents failed", "error", err)
		}
		return ""
	}
	return formatRoster(agents)
}

// formatRoster renders enabled sub-agents into the roster body — "- <slug> —
// <description>" lines (description omitted when blank), sorted by id for
// byte-deterministic output. "" when there are none. Pure (no DB) so it is
// unit-testable.
func formatRoster(agents []agentmodels.Agent) string {
	sorted := append([]agentmodels.Agent(nil), agents...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	lines := make([]string, 0, len(sorted))
	for _, ag := range sorted {
		line := "- " + ag.Slug
		if desc := strings.TrimSpace(ag.Description); desc != "" {
			line += " — " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// composerCapabilityRefs converts the Composer's resolved general-tier
// capability (Slice 5, §2) into the agent.Tools (ToolRef array) + agent.KBScope
// JSON the runner resolves — so the Composer gets its general tools + general KB
// through the SAME resolve path sub-agents use (and the resolver auto-adds
// kb_search when the scope is non-empty). Capability tool slugs are existing
// custom (ai_tools) slugs — enforced at config-save — so each becomes a custom
// ToolRef by id. Empty scope/slugs yields nil (no general capability). Best-
// effort: a slug-lookup failure logs and omits those tools, never blocks the run.
func (a *Activities) composerCapabilityRefs(capability composermodels.Capability) (tools, kbScope json.RawMessage) {
	if len(capability.KBScope.FolderIDs) > 0 || len(capability.KBScope.ArticleIDs) > 0 {
		if b, err := json.Marshal(capability.KBScope); err == nil {
			kbScope = b
		}
	}
	if len(capability.ToolSlugs) == 0 || a.Tools == nil {
		return tools, kbScope
	}
	fetched, err := a.Tools.GetBySlugs(capability.ToolSlugs)
	if err != nil {
		a.Lo.Warn("composer capability: resolve tool slugs failed", "error", err)
		return tools, kbScope
	}
	refs := make([]agentmodels.ToolRef, 0, len(fetched))
	for _, t := range fetched {
		idJSON, mErr := json.Marshal(t.ID)
		if mErr != nil {
			continue
		}
		refs = append(refs, agentmodels.ToolRef{Kind: agentmodels.ToolRefKindCustom, ID: idJSON})
	}
	if len(refs) > 0 {
		if b, err := json.Marshal(refs); err == nil {
			tools = b
		}
	}
	return tools, kbScope
}
