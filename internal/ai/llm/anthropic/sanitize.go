package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/zerodha/logf"

	anthropicshared "github.com/aziz-tb/merio/internal/ai/llm/anthropic/shared"
)

// sanitizeForThinking strips sampling knobs from a MessageNewParams
// that Anthropic does not accept when extended thinking is enabled.
//
// Constraints (per Anthropic API docs, surfaced as 400 errors at the
// request boundary):
//
//   - temperature must be omitted or equal to 1.
//   - top_k must be omitted entirely.
//   - top_p must be omitted or inside [0.95, 1.0].
//
// When a preset sets a value that violates a constraint, we strip it
// silently and emit an INFO log so the suppression is visible without
// adding noise to dashboards — operators configured the preset
// reasonably for non-thinking models; the platform is correcting one
// knob to comply with Anthropic's thinking-mode rules.
//
// Returns params unchanged when thinking is disabled (the SDK union's
// OfEnabled / OfAdaptive both nil), so this is safe to call on every
// request and noop on the non-thinking path.
//
// Pure function: takes params + logger context, returns adjusted
// params. Easy to unit-test without an SDK client.
func sanitizeForThinking(params sdk.MessageNewParams, lo *logf.Logger, model string) sdk.MessageNewParams {
	if !thinkingActive(params.Thinking) {
		return params
	}

	// Temperature: only 1.0 is accepted with thinking. Anything else
	// (including the SDK default 1.0 wrapped in param.NewOpt) gets
	// dropped to "unset" so Anthropic uses its own default. We can't
	// distinguish "preset set 1.0 explicitly" from "default" without
	// inspecting the preset, so we always drop — the API behaves the
	// same either way.
	if !param.IsOmitted(params.Temperature) {
		if v := params.Temperature.Value; v != 1.0 {
			lo.Info("anthropic.thinking.suppress_temperature",
				"model", model,
				"configured", v,
				"reason", "thinking requires temperature=1 or unset",
			)
		}
		params.Temperature = param.Opt[float64]{}
	}

	// top_k: must be omitted.
	if !param.IsOmitted(params.TopK) {
		lo.Info("anthropic.thinking.suppress_top_k",
			"model", model,
			"configured", params.TopK.Value,
			"reason", "thinking requires top_k unset",
		)
		params.TopK = param.Opt[int64]{}
	}

	// top_p: must be omitted or in [0.95, 1.0].
	if !param.IsOmitted(params.TopP) {
		if v := params.TopP.Value; !anthropicshared.TopPAllowedForThinking(v) {
			lo.Info("anthropic.thinking.suppress_top_p",
				"model", model,
				"configured", v,
				"allowed_range", "[0.95, 1.0]",
				"reason", "thinking requires top_p in [0.95, 1.0] or unset",
			)
			params.TopP = param.Opt[float64]{}
		}
	}

	return params
}

// thinkingActive reports whether the Thinking parameter is set to
// either enabled or adaptive. Both variants impose the same sampling
// constraints; only ThinkingConfigDisabled (and the zero union) leave
// the constraints off.
func thinkingActive(tc sdk.ThinkingConfigParamUnion) bool {
	return tc.OfEnabled != nil || tc.OfAdaptive != nil
}
