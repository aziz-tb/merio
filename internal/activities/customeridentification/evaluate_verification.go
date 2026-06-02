package customeridentification

import (
	"context"

	verificationmodels "github.com/aziz-tb/merio/internal/verification/models"
	cimodels "github.com/aziz-tb/merio/internal/workflows/customeridentification"
)

func (a *Activities) EvaluateVerificationActivity(ctx context.Context, in cimodels.EvaluateVerificationInput) (cimodels.EvaluateVerificationOutput, error) {
	if in.CustomerVerification != nil {
		msg := ""
		if in.CustomerVerificationMessage != nil {
			msg = *in.CustomerVerificationMessage
		}
		return cimodels.EvaluateVerificationOutput{
			Level:   *in.CustomerVerification,
			Message: msg,
			Source:  "user_code",
		}, nil
	}
	out, err := a.VerificationMgr.Evaluate(ctx, verificationmodels.EvalInput{
		Channel:        in.Channel,
		Rules:          in.Rules,
		Message:        in.PreparedMessage,
		Customer:       in.Customer,
		Contract:       in.Contract,
		ExtractedAttrs: in.ExtractedAttrs,
	})
	if err != nil {
		return cimodels.EvaluateVerificationOutput{}, err
	}
	return cimodels.EvaluateVerificationOutput{
		Level:   int(out.Level),
		Message: out.Message,
		Source:  out.Source,
		Matched: out.Matched,
		Failed:  out.Failed,
	}, nil
}
