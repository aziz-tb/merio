package aitools

import (
	"errors"

	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// MapDispatchErrorToHTTP translates a dispatcher / runner error into the
// (status, message, error type) triple handlers hand to SendErrorEnvelope.
//
//	ErrToolNotFound                  → 404
//	ErrToolDisabled                  → 403
//	ErrAgentVerificationInsufficient → 409
//	ErrParamsInvalid                 → 400
//	ErrCapabilityFailed              → 422
//	ErrImplInvalid                   → 500
//	ExecRunner sandbox-unavailable   → 503
//	ExecRunner invalid-request       → 400
//	unrecognized                     → handled=false; caller logs + 500s
//
// handled reports whether err was a known sentinel; callers log the raw
// error only when it isn't. The agent runner is a second consumer (see
// internal/ai/runner/runner.go), so the mapping lives next to the sentinels.
func MapDispatchErrorToHTTP(err error) (status int, message string, etype fastglue.ErrorType, handled bool) {
	switch {
	case errors.Is(err, ErrToolNotFound):
		return fasthttp.StatusNotFound, "AI tool not found", envelope.InputError, true
	case errors.Is(err, ErrToolDisabled):
		return fasthttp.StatusForbidden, "AI tool is disabled", envelope.PermissionError, true
	case errors.Is(err, ErrAgentVerificationInsufficient):
		return fasthttp.StatusConflict,
			"conversation verification level is below the agent requirement",
			envelope.ConflictError, true
	case errors.Is(err, ErrParamsInvalid):
		return fasthttp.StatusBadRequest, err.Error(), envelope.InputError, true
	case errors.Is(err, ErrCapabilityFailed):
		return fasthttp.StatusUnprocessableEntity, err.Error(), envelope.GeneralError, true
	case errors.Is(err, ErrImplInvalid):
		return fasthttp.StatusInternalServerError, err.Error(), envelope.GeneralError, true
	case IsRunnerErrSandboxUnavailable(err):
		return fasthttp.StatusServiceUnavailable,
			"sandbox temporarily unavailable",
			envelope.GeneralError, true
	case IsRunnerErrInvalidRequest(err):
		return fasthttp.StatusBadRequest, err.Error(), envelope.InputError, true
	default:
		return 0, "", "", false
	}
}
