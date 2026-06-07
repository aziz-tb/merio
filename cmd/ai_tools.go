package main

import (
	"encoding/json"
	"strconv"
	"strings"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	aitools "github.com/aziz-tb/merio/internal/ai/tools"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	toolmodels "github.com/aziz-tb/merio/internal/ai/tools/models"
	amodels "github.com/aziz-tb/merio/internal/auth/models"
	"github.com/aziz-tb/merio/internal/envelope"
	execModels "github.com/aziz-tb/merio/internal/exec/runner/models"
	fnmodels "github.com/aziz-tb/merio/internal/function/models"
	"github.com/aziz-tb/merio/internal/stringutil"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// handleListAITools lists ai tools with optional `enabled` filter and
// pagination (page, page_size).
func handleListAITools(r *fastglue.Request) error {
	var (
		app                  = r.Context.(*App)
		enabledQ             = r.RequestCtx.QueryArgs().Peek("enabled")
		page, pageSize       = getPagination(r)
		enabled        *bool = nil
	)
	if len(enabledQ) > 0 {
		v := strings.EqualFold(string(enabledQ), "true")
		enabled = &v
	}
	out, total, err := app.aiTools.List(enabled, pageSize, (page-1)*pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]interface{}{
		"results":   out,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// handleGetAITool fetches one ai tool by ID.
func handleGetAITool(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	out, fetchErr := app.aiTools.Get(id)
	if fetchErr != nil {
		return sendErrorEnvelope(r, fetchErr)
	}
	return r.SendEnvelope(out)
}

// handleCreateAITool creates an ai tool. The request body carries the same
// shape as before the Unified Executor Framework batch — including
// implementation:{code,language,packages} — but storage is now split: the
// code lands in a functions row (domain='ai_tool', name=slug) and the
// ai_tools row links to it via function_id. The split is invisible to the
// frontend.
func handleCreateAITool(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		tool = toolmodels.Tool{}
	)
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	if err := r.Decode(&tool, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := validateAITool(&tool); err != nil {
		return sendErrorEnvelope(r, err)
	}

	// 1. Create the functions row (the code blob). name = slug; the
	//    dispatcher runs the tool by slug.
	code, lang, pkgs := splitImplementation(tool.Implementation)
	svc := aitools.NewService(app.db, app.aiTools, app.function)
	out, err := svc.CreateToolWithFunction(r.RequestCtx, tool, fnmodels.Function{
		Name:            tool.Slug,
		Domain:          fnmodels.DomainAITool,
		Code:            code,
		Language:        lang,
		Packages:        pkgs,
		TimeoutMS:       tool.TimeoutMS,
		UpdatedByUserID: nullIntFromUser(user.ID),
	})
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Reconstruct the implementation view for the response.
	out.Implementation = buildImplementationJSON(code, lang, pkgs)
	return r.SendEnvelope(out)
}

// handleUpdateAITool updates an ai tool. Same validation as Create; updates
// both the functions row (code) and the ai_tools row (metadata).
func handleUpdateAITool(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		tool    = toolmodels.Tool{}
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if err := r.Decode(&tool, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	// Reject slug mutations — slug is a stable code reference (the
	// dispatcher resolves tools by slug; agents reference tools by it
	// inside their tools[] JSONB; the linked function's name == slug).
	existing, fetchErr := app.aiTools.Get(id)
	if fetchErr != nil {
		return sendErrorEnvelope(r, fetchErr)
	}
	if strings.TrimSpace(tool.Slug) != "" && tool.Slug != existing.Slug {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, stringutil.ErrSlugImmutableMessage, nil, envelope.InputError)
	}
	tool.Slug = existing.Slug
	if vErr := validateAITool(&tool); vErr != nil {
		return sendErrorEnvelope(r, vErr)
	}

	// 1. Update the linked functions row (name is immutable == slug).
	code, lang, pkgs := splitImplementation(tool.Implementation)
	tool.FunctionID = existing.FunctionID
	svc := aitools.NewService(app.db, app.aiTools, app.function)
	out, updErr := svc.UpdateToolWithFunction(r.RequestCtx, id, existing.Slug, tool, fnmodels.Function{
		Code:            code,
		Language:        lang,
		Packages:        pkgs,
		TimeoutMS:       tool.TimeoutMS,
		UpdatedByUserID: nullIntFromUser(user.ID),
	})
	if updErr != nil {
		return sendErrorEnvelope(r, updErr)
	}
	out.Implementation = buildImplementationJSON(code, lang, pkgs)
	return r.SendEnvelope(out)
}

// handleDeleteAITool hard-deletes an ai tool by ID, then its linked
// functions row (the FK is RESTRICT, so the tool must go first).
func handleDeleteAITool(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	existing, fetchErr := app.aiTools.Get(id)
	if fetchErr != nil {
		return sendErrorEnvelope(r, fetchErr)
	}
	svc := aitools.NewService(app.db, app.aiTools, app.function)
	if delErr := svc.DeleteToolWithFunction(r.RequestCtx, id, existing.Slug); delErr != nil {
		return sendErrorEnvelope(r, delErr)
	}
	return r.SendEnvelope(true)
}

// invokeAIToolReq is the request body for POST /api/v1/ai/tools/{slug}/invoke.
//
// Manual invocation: the caller fills `parameters` directly. `conversation_id`
// is plumbed to the runner / runtime context so SDK callbacks and capabilities
// reading conversation scope get default-scoping. `verification_level` is a
// manual-test affordance for internal capabilities that shape PII by
// conversation verification state. `kind` defaults to "custom" for back-compat
// with F9 callers.
type invokeAIToolReq struct {
	Kind              string         `json:"kind"`
	Parameters        map[string]any `json:"parameters"`
	ConversationID    int            `json:"conversation_id"`
	VerificationLevel int            `json:"verification_level"`
}

// handleInvokeAITool dispatches a python_script tool through the F6
// sandbox. The request body is the parameter map; the response on success
// is the runner's Result (kind / data / message / telemetry).
//
// HTTP status mapping:
//   - 200: tool ran (kind in body distinguishes success/failure/noop/error)
//   - 400: parameter validation failed, malformed body, malformed tool impl
//   - 403: tool disabled, wrong type (go_builtin), agent verification insufficient
//   - 404: tool slug not found
//   - 422: script raised an uncaught exception (sandbox returned KindError)
//   - 503: sandbox circuit breaker open
//
// HTTP 422 vs 200-with-kind=error: the Chunk 6 contract maps Python
// exceptions to 422 specifically. The runner returns Result.Kind=error
// for both Python exceptions AND infrastructure errors; we use Result.Code
// to distinguish: codes prefixed with "PY_" come from script-raised
// exceptions and map to 422; everything else stays 200 (the script
// emitted noop/failure deliberately) or 503 (infra).
func handleInvokeAITool(r *fastglue.Request) error {
	app := r.Context.(*App)

	slug := r.RequestCtx.UserValue("slug").(string)
	if slug == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	body := invokeAIToolReq{}
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if body.Parameters == nil {
		body.Parameters = map[string]any{}
	}

	// Caller identity from session/API-key auth (set by perm middleware).
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)

	// kind defaults to "custom" for back-compat. The dispatcher does the
	// same defaulting; we set it here so the request shape that hits the
	// runtime context is explicit.
	kind := agentmodels.ToolRefKind(body.Kind)
	if kind == "" {
		kind = agentmodels.ToolRefKindCustom
	}
	if kind != agentmodels.ToolRefKindCustom && kind != agentmodels.ToolRefKindInternal {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			"kind must be \"custom\" or \"internal\"", nil, envelope.InputError)
	}

	// Internal capabilities need access to the App's manager handles
	// (knowledge, conversation, user). The runtime context is the
	// channel the resolved capability reads them through.
	dispatchReq := aitools.DispatchRequest{
		Kind:              kind,
		Slug:              slug,
		InvokerUserID:     user.ID,
		VerificationLevel: body.VerificationLevel,
		Parameters:        body.Parameters,
		ConversationID:    body.ConversationID,
	}
	if kind == agentmodels.ToolRefKindInternal {
		dispatchReq.RuntimeCtx = &builtin.ToolRuntimeContext{
			ConversationID:    body.ConversationID,
			VerificationLevel: body.VerificationLevel,
			Logf:              app.lo,
			Knowledge:         app.knowledge,
			Conversation:      app.conversation,
			User:              app.user,
			Customer:          app.customer,
			Contract:          app.contract,
		}
	}

	result, err := app.aiToolDispatcher.Dispatch(r.RequestCtx, dispatchReq)
	if err != nil {
		if status, msg, etype, handled := aitools.MapDispatchErrorToHTTP(err); handled {
			return r.SendErrorEnvelope(status, msg, nil, etype)
		}
		app.lo.Error("ai tool dispatch failed", "slug", slug, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError,
			app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Map runner result kind to HTTP status using the EXECUTION_* enum
	// the runner now emits (see internal/exec/runner/models/models.go):
	//   success/failure/noop      → 200 (kind in body distinguishes)
	//   error + EXECUTION_TIMEOUT → 408 (script ran past its declared timeout)
	//   error + EXECUTION_EXCEPTION → 422 (Python exception; stderr has the traceback)
	//   error + EXECUTION_FAILED  → 500 (script crashed for a non-traceback reason)
	//   error + anything else     → 500 (forward-compat for new codes)
	if result != nil && result.Kind == "error" {
		switch result.Code {
		case execModels.CodeExecutionTimeout:
			return r.SendErrorEnvelope(fasthttp.StatusRequestTimeout, result.Message, result, envelope.GeneralError)
		case execModels.CodeExecutionException:
			return r.SendErrorEnvelope(fasthttp.StatusUnprocessableEntity, result.Message, result, envelope.GeneralError)
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, result.Message, result, envelope.GeneralError)
	}
	return r.SendEnvelope(result)
}

// validateAITool enforces validation rules for ai_tools rows after the
// F5 schema cleanup: name required, slug auto-generated or shape-checked,
// implementation JSONB is a valid PythonScriptImpl, and category is one
// of read/write. ai_tools holds only custom tools; internal capabilities
// live in code under internal/ai/tools/builtin/.
func validateAITool(t *toolmodels.Tool) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return envelope.NewError(envelope.InputError, "name is required", nil)
	}

	t.Slug = strings.TrimSpace(t.Slug)
	if t.Slug == "" {
		t.Slug = stringutil.Slugify(t.Name)
		if t.Slug == "" {
			return envelope.NewError(envelope.InputError, "could not derive slug from name; provide one explicitly", nil)
		}
	}
	if !stringutil.SlugRegex.MatchString(t.Slug) {
		return envelope.NewError(envelope.InputError, "slug must match ^[a-z0-9-]+$ with no leading/trailing/double dashes", nil)
	}

	// Cap matches the functions table's check_functions_timeout (the code
	// now lives in a functions row, which is the binding constraint).
	if t.TimeoutMS <= 0 || t.TimeoutMS > 120000 {
		return envelope.NewError(envelope.InputError, "timeout_ms must be between 1 and 120000", nil)
	}
	t.Category = strings.ToLower(strings.TrimSpace(t.Category))
	if t.Category == "" {
		t.Category = "write"
	}
	if t.Category != "read" && t.Category != "write" {
		return envelope.NewError(envelope.InputError, "category must be 'read' or 'write'", nil)
	}
	// Implementation JSONB: custom tools dispatch to the F6 exec runner,
	// which supports python + javascript (the JS lane runs without the
	// SDK in v1 — admins compose stateless transforms there). Validate
	// the shape, require non-empty code, and lock the language to the
	// two values the runner whitelists at dispatch time (runner.go:269)
	// so a bad save can't get past the CRUD into a "saves fine, fails
	// at run time" loop.
	if len(t.Implementation) > 0 && string(t.Implementation) != "{}" && string(t.Implementation) != "null" {
		var impl toolmodels.PythonScriptImpl
		if err := json.Unmarshal(t.Implementation, &impl); err != nil {
			return envelope.NewError(envelope.InputError, "implementation must be {code, language, packages}", nil)
		}
		if strings.TrimSpace(impl.Code) == "" {
			return envelope.NewError(envelope.InputError, "implementation.code is required", nil)
		}
		lang := strings.ToLower(strings.TrimSpace(impl.Language))
		if lang == "" {
			lang = "python"
			impl.Language = "python"
		}
		if lang != "python" && lang != "javascript" {
			return envelope.NewError(envelope.InputError,
				"implementation.language must be \"python\" or \"javascript\"", nil)
		}
		// Re-marshal so the canonical lowercase language lands in the row.
		canonical, err := json.Marshal(impl)
		if err != nil {
			return envelope.NewError(envelope.GeneralError, "could not normalize tool implementation", nil)
		}
		t.Implementation = canonical
	}

	return nil
}

// splitImplementation parses the implementation JSONB ({code,language,
// packages}) into its parts for the linked functions row. validateAITool
// has already normalized language to "python"/"javascript" when an
// implementation is present; an absent implementation yields empty code
// with the default language so the functions language CHECK is satisfied.
func splitImplementation(raw json.RawMessage) (code, language, packages string) {
	language = "python"
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return "", language, ""
	}
	var impl toolmodels.PythonScriptImpl
	if err := json.Unmarshal(raw, &impl); err != nil {
		return "", language, ""
	}
	if l := strings.ToLower(strings.TrimSpace(impl.Language)); l != "" {
		language = l
	}
	return impl.Code, language, impl.Packages
}

// buildImplementationJSON reconstructs the implementation view for the
// admin API response so the frontend sees the same shape as before.
func buildImplementationJSON(code, language, packages string) json.RawMessage {
	b, _ := json.Marshal(toolmodels.PythonScriptImpl{Code: code, Language: language, Packages: packages})
	return b
}

// nullIntFromUser returns a null.Int for a real user id, NULL for 0/system.
func nullIntFromUser(id int) null.Int {
	if id <= 0 {
		return null.Int{}
	}
	return null.IntFrom(id)
}
