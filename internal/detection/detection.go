// Package detection implements the universal detection layer used by
// automation rules and tags. It owns the AST, recursive evaluator, path
// resolver, and AI classifier hook.
package detection

import (
	"context"
	"fmt"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/aziz-tb/merio/internal/ai/usage"
	"github.com/zerodha/logf"
)

const defaultClassificationPreset = "fast-haiku"

// Manager is the public entry point for detection evaluation.
type Manager struct {
	lo            *logf.Logger
	llmProvider   llm.LLMProvider
	presetSlug    string
	usageMgr      *usage.Manager
	systemInboxID int
}

type Opts struct {
	Lo          *logf.Logger
	LLMProvider llm.LLMProvider
	PresetSlug  string
	// UsageMgr + SystemInboxID enable ai_usage recording for AI-mode
	// detection (Batch 9 audit-gap fix). Both optional: a nil UsageMgr
	// or zero inbox id skips the write (the evaluation still runs).
	UsageMgr      *usage.Manager
	SystemInboxID int
}

func New(opts Opts) *Manager {
	presetSlug := opts.PresetSlug
	if presetSlug == "" {
		presetSlug = defaultClassificationPreset
	}
	return &Manager{
		lo:            opts.Lo,
		llmProvider:   opts.LLMProvider,
		presetSlug:    presetSlug,
		usageMgr:      opts.UsageMgr,
		systemInboxID: opts.SystemInboxID,
	}
}

// Evaluate runs a detection against the context.
func (m *Manager) Evaluate(ctx context.Context, det Detection, ec EvaluationContext) (EvaluationResult, error) {
	switch det.Mode {
	case ModeManualOnly:
		return EvaluationResult{Reasoning: "manual_only mode"}, nil
	case ModeRulesOnly, "":
		if det.Rules == nil {
			return EvaluationResult{Reasoning: "no rules defined"}, nil
		}
		matched := evaluateRules(det.Rules, ec, m.lo)
		return EvaluationResult{Matched: matched, Score: boolToFloat(matched), Reasoning: rulesReasoning(matched)}, nil
	case ModeAIOnly:
		if det.AI == nil || m.llmProvider == nil {
			return EvaluationResult{Reasoning: "no AI configured"}, nil
		}
		return m.evaluateAI(ctx, det.AI, ec)
	case ModeRulesAndAI:
		if det.Rules == nil || det.AI == nil {
			return EvaluationResult{Reasoning: "incomplete config for rules_and_ai"}, nil
		}
		if !evaluateRules(det.Rules, ec, m.lo) {
			return EvaluationResult{Reasoning: "rules failed"}, nil
		}
		if m.llmProvider == nil {
			return EvaluationResult{Reasoning: "rules passed but no LLM provider"}, nil
		}
		return m.evaluateAI(ctx, det.AI, ec)
	case ModeRulesOrAI:
		if det.Rules != nil && evaluateRules(det.Rules, ec, m.lo) {
			return EvaluationResult{Matched: true, Score: 1.0, Reasoning: "rules matched"}, nil
		}
		if det.AI != nil && m.llmProvider != nil {
			return m.evaluateAI(ctx, det.AI, ec)
		}
		return EvaluationResult{Reasoning: "rules failed, no AI configured"}, nil
	default:
		return EvaluationResult{}, fmt.Errorf("unknown mode: %s", det.Mode)
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func rulesReasoning(matched bool) string {
	if matched {
		return "rules matched"
	}
	return "rules failed"
}
