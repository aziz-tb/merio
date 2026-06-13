package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/zerodha/logf"

	anthropicshared "github.com/aziz-tb/merio/internal/ai/llm/anthropic/shared"
)

// sanitizeForThinking strips sampling knobs Anthropic rejects (with a 400)
// when extended thinking is enabled, logging each suppression at INFO:
//
//   - temperature must be omitted or equal to 1.
//   - top_k must be omitted entirely.
//   - top_p must be omitted or inside [0.95, 1.0].
//
// Returns params unchanged when thinking is disabled, so it is safe to call
// on every request.
func sanitizeForThinking(params sdk.MessageNewParams, lo *logf.Logger, model string) sdk.MessageNewParams {
	if !thinkingActive(params.Thinking) {
		return params
	}

	// Temperature: drop to unset whenever present. We can't tell an explicit
	// 1.0 from the SDK default, and dropping it makes Anthropic use its own
	// default (1.0), so the result is identical either way.
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
