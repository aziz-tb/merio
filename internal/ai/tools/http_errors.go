package aitools

import (
	"errors"

	"github.com/aziz-tb/merio/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// MapDispatchErrorToHTTP translates a dispatcher / runner error into the
// triple every HTTP handler in the project hands to SendErrorEnvelope:
// status code, human message, error type.
//
//	ErrToolNotFound                  → 404
//	ErrToolDisabled                  → 403
//	ErrAgentVerificationInsufficient → 409
//	ErrParamsInvalid                 → 400
//	ErrCapabilityFailed              → 422 (in-process Go error; analogous to a
//	                                         Python exception surfaced from the sandbox)
//	ErrImplInvalid                   → 500
//	ExecRunner sandbox-unavailable → 503
//	ExecRunner invalid-request     → 400
//	unrecognized                   → handled=false; caller logs + 500s
//
// The bool return reports whether the error was a known dispatcher /
// runner sentinel. Callers use it to decide whether to log the raw
// error (unknown → log) vs. trust the typed mapping (known → no log,
// the status code already tells the story).
//
// The agent runner is a second real consumer of this mapping (see
// internal/ai/runner/runner.go) — keeping it next to the dispatcher
// sentinels means a new sentinel and its HTTP mapping land in one PR
// instead of drifting between handler and runtime.
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
