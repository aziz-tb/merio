package airuns

import (
	"context"

	"github.com/aziz-tb/merio/internal/composer"
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
