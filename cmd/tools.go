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

// toolRegistry looks up a tool by name. Until the AI registry exists,
// app.toolRegistry is nil and every call to the route returns 404.
type toolRegistry interface {
	// LoadTool returns the tool's runtime definition, or an error wrapping
	// errToolNotFound if the name doesn't exist.
	LoadTool(name string) (*toolDefinition, error)
}

// toolDefinition is the shape tool execution needs from the registry.
type toolDefinition struct {
	Code           string
	Language       string
	Packages       []string
	AllowedDomains []string
	TimeoutSeconds int
	MemoryMB       int
}

// errToolNotFound is the sentinel toolRegistry returns when a name isn't
// in the registry.
var errToolNotFound = errors.New("tool not found")

// handleToolCall implements POST /api/v1/tools/{name}/call. Script-only:
// session and non-script callers are rejected.
func handleToolCall(r *fastglue.Request) error {
	app := r.Context.(*App)

	// The perm decorator authenticated some identity; refuse any that isn't
	// a script token.
	if r.RequestCtx.UserValue("auth_method") != "script_token" {
		return r.SendErrorEnvelope(
			http.StatusUnauthorized,
			app.i18n.T("auth.scriptTokenRequired"),
			nil,
			envelope.PermissionError,
		)
	}

	// Execution metadata stashed by the auth middleware.
	tok, ok := r.RequestCtx.UserValue("script_token").(*execTokenModels.Token)
	if !ok || tok == nil {
		// auth_method=script_token implies the token is set; defensive 500.
		app.lo.Error("script_token missing from context despite auth_method=script_token")
		return r.SendErrorEnvelope(
			http.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"),
			nil,
			envelope.GeneralError,
		)
	}

	// Server-side depth check: the SDK also checks locally, but a script
	// could tamper with the env var it reads.
	if tok.ToolDepth >= execRunner.MaxToolDepth {
		return r.SendErrorEnvelope(
			http.StatusForbidden,
			app.i18n.T("exec.depthLimitExceeded"),
			map[string]string{"code": "DEPTH_LIMIT_EXCEEDED"},
			envelope.PermissionError,
		)
	}

	toolName := r.RequestCtx.UserValue("name").(string)

	// No registry wired in yet: 404, surfaced by the SDK as NotFoundError.
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

	// Inherit the caller's identity (invoker, conversation, trace_id) so
	// child execution attribution chains correctly.
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

	// HTTP 200 regardless of result.Kind: the SDK reads .kind from the body
	// to decide what to do. HTTP errors are reserved for infra failures.
	return r.SendEnvelope(result)
}
