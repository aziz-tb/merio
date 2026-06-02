package customeridentification

import (
	"context"
	"strings"

	cimodels "github.com/aziz-tb/merio/internal/workflows/customeridentification"
)

func (a *Activities) ShouldRunActivity(ctx context.Context, in cimodels.ShouldRunInput) (cimodels.ShouldRunOutput, error) {
	_ = ctx
	if strings.ToLower(strings.TrimSpace(in.Channel)) != "email" {
		return cimodels.ShouldRunOutput{ShouldRun: false, Reason: "channel not supported"}, nil
	}
	if !in.Settings.PatternDetectionEnabled && !in.Settings.LLMExtractionEnabled {
		return cimodels.ShouldRunOutput{ShouldRun: false, Reason: "customer identification disabled"}, nil
	}
	return cimodels.ShouldRunOutput{ShouldRun: true}, nil
}
