// Package verification decides whether a recognized customer is "verified"
// against a conversation's message context. Evaluate runs the group/min_count
// rules and returns a level (0/1/2), a human-readable reason, and a
// per-criterion trace.
//
// Evaluate is a deterministic pure function with no settings, DB, Temporal, or
// HTTP dependency: the caller pre-loads the rules and passes them in via
// EvalInput.
package verification

import (
	"context"
	"strings"

	"github.com/aziz-tb/merio/internal/verification/models"
	"github.com/zerodha/logf"
)

// Manager evaluates verification rules. It holds no data dependencies — the
// optional logger is used only for diagnostics.
type Manager struct {
	lo *logf.Logger
}

// Opts wires the (optional) logger into the Manager.
type Opts struct {
	Lo *logf.Logger
}

// New returns a new Manager.
func New(opts Opts) *Manager {
	return &Manager{lo: opts.Lo}
}

// Evaluate runs the channel's verification rules against the input and returns
// the level, message, and trace. It never returns level 3 (operator-set only).
func (m *Manager) Evaluate(_ context.Context, in models.EvalInput) (models.EvalOutput, error) {
	out := models.EvalOutput{Level: models.VerificationLevelUnverified, Source: "rules"}

	channel, ok := in.Rules.ByChannel(in.Channel)
	if !ok || len(channel.Groups) == 0 {
		// No ruleset for this channel means there is nothing to verify against.
		return out, nil
	}

	cctx := models.CriterionContext{
		Message:        in.Message,
		Customer:       in.Customer,
		Contract:       in.Contract,
		ExtractedAttrs: in.ExtractedAttrs,
	}

	matchedAny := false
	allSatisfied := true
	var partialGroups []partialGroupMessage

	for _, g := range channel.Groups {
		customByLabel := make(map[string]models.CustomCriterion, len(g.CustomCriteria))
		for _, c := range g.CustomCriteria {
			customByLabel[c.Label] = c
		}

		matchedInGroup := 0
		var groupMatched []models.CriterionResult
		var groupFailed []models.CriterionResult

		for _, crit := range g.Criteria {
			res := models.CriterionResult{
				Type:       crit.Type,
				Name:       crit.Name,
				Label:      crit.Label,
				GroupLabel: g.Label,
			}

			matched := false
			if crit.Type == "custom" {
				if cc, found := customByLabel[crit.Label]; found {
					matched = EvaluateCustom(cc, cctx)
					if !matched {
						res.Reason = renderReason(cc, cctx)
					}
				} else if m.lo != nil {
					// A rule referencing an undefined custom label fails
					// closed; surface it for the operator's diagnostics.
					m.lo.Debug("verification: rule references undefined custom criterion", "label", crit.Label)
				}
			} else {
				matched = EvaluateStandard(crit.Name, cctx)
			}

			if matched {
				matchedInGroup++
				matchedAny = true
				out.Matched = append(out.Matched, res)
				groupMatched = append(groupMatched, res)
			} else {
				out.Failed = append(out.Failed, res)
				groupFailed = append(groupFailed, res)
			}
		}

		if matchedInGroup < g.MinCount {
			allSatisfied = false
			partialGroups = append(partialGroups, partialGroupMessage{
				Label:      g.Label,
				Required:   g.MinCount,
				Matched:    matchedInGroup,
				Successful: groupMatched,
				Failed:     groupFailed,
			})
		}
	}

	switch {
	case allSatisfied:
		out.Level = models.VerificationLevelVerified
	case matchedAny:
		out.Level = models.VerificationLevelPartial
	default:
		out.Level = models.VerificationLevelUnverified
	}

	// Only partial carries a message: an operator-facing Markdown explanation.
	if out.Level == models.VerificationLevelPartial {
		out.Message = buildPartialVerificationMessage(partialGroups, cctx)
	}
	return out, nil
}

// renderReason substitutes {value} in a custom criterion's reason with the
// value resolved at its key. Missing value → empty substitution.
func renderReason(cc models.CustomCriterion, ctx models.CriterionContext) string {
	if cc.Reason == "" {
		return ""
	}
	val, _ := resolveCustomValue(cc, ctx)
	return strings.ReplaceAll(cc.Reason, "{value}", val)
}
