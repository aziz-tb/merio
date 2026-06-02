package main

import (
	"errors"
	"net/http"

	"github.com/aziz-tb/merio/internal/envelope"
	execRunner "github.com/aziz-tb/merio/internal/exec/runner"
	execRunnerModels "github.com/aziz-tb/merio/internal/exec/runner/models"
	execTokenModels "github.com/aziz-tb/merio/internal/exec/token/models"
	"github.com/zerodha/fastglue"
)

// toolRegistry is the minimal interface this handler needs to look up a
// tool by name. The full registry is implemented in `internal/ai` (the
// AI infrastructure plan).
//
// Stub semantics for v1: until the AI registry exists, app.toolRegistry
// is nil and every call to /api/v1/tools/:name/call returns 404. The
// route is wired but inert.
type toolRegistry interface {
	// LoadTool returns the runtime definition for a registered tool, or
	// an error wrapping ErrToolNotFound if the name doesn't exist.
	LoadTool(name string) (*toolDefinition, error)
}

// toolDefinition is the minimal shape Tool execution needs from the
// registry. The real type lives in `internal/ai/models` (TBD); the
// handler doesn't care about extra fields the registry tracks.
type toolDefinition struct {
	Code           string
	Language       string
	Packages       []string
	AllowedDomains []string
	TimeoutSeconds int
	MemoryMB       int
}

// errToolNotFound is the sentinel toolRegistry returns when a name isn't
// in the registry. Defined here as a sentinel, but the actual registry
// implementation will export its own version of this and we'll switch
// to errors.Is when wired up.
var errToolNotFound = errors.New("tool not found")

// handleToolCall implements POST /api/v1/tools/{name}/call.
//
// This is the only endpoint reserved for script-only access. Session
// callers are rejected — there's no user-facing reason to invoke a tool
// by name outside of a running script.
func handleToolCall(r *fastglue.Request) error {
	app := r.Context.(*App)

	// Reject non-script callers. The `perm` decorator already authenticated
	// SOMETHING (script token, API key, or session), but we want to refuse
	// non-script identities reaching this endpoint.
	if r.RequestCtx.UserValue("auth_method") != "script_token" {
		return r.SendErrorEnvelope(
			http.StatusUnauthorized,
			app.i18n.T("auth.scriptTokenRequired"),
			nil,
			envelope.PermissionError,
		)
	}

	// Pull execution metadata that the auth middleware stashed on the
	// request context.
	tok, ok := r.RequestCtx.UserValue("script_token").(*execTokenModels.Token)
	if !ok || tok == nil {
		// Should never happen — auth_method=script_token implies the
		// token is set. Defensive 500 if it isn't.
		app.lo.Error("script_token missing from context despite auth_method=script_token")
		return r.SendErrorEnvelope(
			http.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"),
			nil,
			envelope.GeneralError,
		)
	}

	// Server-side depth check. The SDK catches DepthLimitError locally
	// before the HTTP call, but we double-check here in case a script
	// tampered with the env var the SDK reads.
	if tok.ToolDepth >= execRunner.MaxToolDepth {
		return r.SendErrorEnvelope(
			http.StatusForbidden,
			app.i18n.T("exec.depthLimitExceeded"),
			map[string]string{"code": "DEPTH_LIMIT_EXCEEDED"},
			envelope.PermissionError,
		)
	}

	toolName := r.RequestCtx.UserValue("name").(string)

	// Look up the tool. With no registry wired in, this fails loudly and
	// the script gets a 404 — the SDK surfaces it as NotFoundError.
	if app.toolRegistry == nil {
		return r.SendErrorEnvelope(
			http.StatusNotFound,
			app.i18n.T("exec.toolRegistryNotConfigured"),
			nil,
			envelope.GeneralError,
		)
	}
	tool, err := app.toolRegistry.LoadTool(toolName)
	if err != nil {
		if errors.Is(err, errToolNotFound) {
			return r.SendErrorEnvelope(
				http.StatusNotFound,
				app.i18n.T("exec.toolNotFound"),
				nil,
				envelope.GeneralError,
			)
		}
		return sendErrorEnvelope(r, err)
	}

	// Parse args from request body.
	var body struct {
		Args map[string]any `json:"args"`
	}
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(
			http.StatusBadRequest,
			app.i18n.T("globals.messages.invalidJSON"),
			nil,
			envelope.InputError,
		)
	}

	// Dispatch. Inherit the caller's identity (invoker, conversation,
	// trace_id) so child execution attribution chains correctly.
	result, err := app.execRunner.Execute(r.RequestCtx, execRunnerModels.Request{
		InvokerUserID:  tok.InvokerUserID,
		Code:           tool.Code,
		Language:       tool.Language,
		Packages:       tool.Packages,
		AllowedDomains: tool.AllowedDomains,
		TimeoutSeconds: tool.TimeoutSeconds,
		MemoryMB:       tool.MemoryMB,
		Input:          body.Args,
		ConversationID: tok.ConversationID,
		ToolName:       toolName,
		ToolDepth:      tok.ToolDepth + 1,
		TraceID:        tok.TraceID,
	})
	if err != nil {
		if errors.Is(err, execRunner.ErrSandboxUnavailable) {
			return r.SendErrorEnvelope(
				http.StatusServiceUnavailable,
				app.i18n.T("exec.sandboxUnavailable"),
				nil,
				envelope.GeneralError,
			)
		}
		if errors.Is(err, execRunner.ErrDepthLimitExceeded) {
			return r.SendErrorEnvelope(
				http.StatusForbidden,
				app.i18n.T("exec.depthLimitExceeded"),
				map[string]string{"code": "DEPTH_LIMIT_EXCEEDED"},
				envelope.PermissionError,
			)
		}
		return sendErrorEnvelope(r, err)
	}

	// Return the result envelope to the calling script. HTTP 200 here
	// regardless of result.Kind — the script's SDK reads .kind from the
	// body and decides what to do (e.g., raise on "error" or use the
	// data on "success"). HTTP errors are reserved for infrastructure
	// failures.
	return r.SendEnvelope(result)
}
