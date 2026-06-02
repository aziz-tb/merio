package main

import (
	"encoding/json"
	"errors"
	"strings"

	aitools "github.com/abhinavxd/libredesk/internal/ai/tools"
	amodels "github.com/abhinavxd/libredesk/internal/auth/models"
	"github.com/abhinavxd/libredesk/internal/envelope"
	execrunner "github.com/abhinavxd/libredesk/internal/exec/runner"
	execModels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
	"github.com/abhinavxd/libredesk/internal/function"
	fnmodels "github.com/abhinavxd/libredesk/internal/function/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// functionErrorToHTTP maps function-package sentinels to envelope replies.
// Returns true when handled.
func functionErrorToHTTP(r *fastglue.Request, err error) (bool, error) {
	app := r.Context.(*App)
	switch {
	case errors.Is(err, function.ErrFunctionNotFound):
		return true, r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.notFound"), nil, envelope.InputError)
	case errors.Is(err, function.ErrFunctionProtected):
		return true, r.SendErrorEnvelope(fasthttp.StatusForbidden, "this function is managed by the platform and cannot be modified here", nil, envelope.PermissionError)
	}
	return false, nil
}

// handleListFunctions lists functions, optionally filtered by ?domain=.
func handleListFunctions(r *fastglue.Request) error {
	app := r.Context.(*App)
	domain := string(r.RequestCtx.QueryArgs().Peek("domain"))
	out, err := app.function.ListByDomain(domain)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(out)
}

// handleGetFunction loads one function by name.
func handleGetFunction(r *fastglue.Request) error {
	app := r.Context.(*App)
	name := r.RequestCtx.UserValue("name").(string)
	if strings.TrimSpace(name) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	out, err := app.function.GetByName(name)
	if err != nil {
		if handled, resp := functionErrorToHTTP(r, err); handled {
			return resp
		}
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(out)
}

// updateFunctionReq is the PUT body. Name and domain are immutable and not
// accepted here.
type updateFunctionReq struct {
	Code       string          `json:"code"`
	Language   string          `json:"language"`
	Packages   string          `json:"packages"`
	TimeoutMS  int             `json:"timeout_ms"`
	Parameters json.RawMessage `json:"parameters"`
}

// handleUpdateFunction updates a function's code/language/packages/timeout/
// parameters by name.
func handleUpdateFunction(r *fastglue.Request) error {
	app := r.Context.(*App)
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	name := r.RequestCtx.UserValue("name").(string)
	if strings.TrimSpace(name) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	var body updateFunctionReq
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	lang := strings.ToLower(strings.TrimSpace(body.Language))
	if lang == "" {
		lang = "python"
	}
	if lang != "python" && lang != "javascript" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "language must be \"python\" or \"javascript\"", nil, envelope.InputError)
	}
	if body.TimeoutMS <= 0 || body.TimeoutMS > 120000 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "timeout_ms must be between 1 and 120000", nil, envelope.InputError)
	}
	err := app.function.Update(name, &fnmodels.Function{
		Code:            body.Code,
		Language:        lang,
		Packages:        body.Packages,
		TimeoutMS:       body.TimeoutMS,
		Parameters:      body.Parameters,
		UpdatedByUserID: nullIntFromUser(user.ID),
	})
	if err != nil {
		if handled, resp := functionErrorToHTTP(r, err); handled {
			return resp
		}
		return sendErrorEnvelope(r, err)
	}
	out, err := app.function.GetByName(name)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(out)
}

// previewFunctionReq is the POST /function/preview body. The code here
// overrides the saved code so operators can test unsaved edits.
type previewFunctionReq struct {
	ID         int64                  `json:"id"`
	Code       string                 `json:"code"`
	Language   string                 `json:"language"`
	Packages   string                 `json:"packages"`
	Parameters []previewFunctionParam `json:"parameters"`
}

type previewFunctionParam struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// handlePreviewFunction test-runs a function with the supplied (possibly
// unsaved) code + parameters and returns the full runner Result. The audit
// row's tool_name is prefixed "function-preview:" so preview runs are
// distinguishable from real runs in function_executions history.
func handlePreviewFunction(r *fastglue.Request) error {
	app := r.Context.(*App)
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	name := string(r.RequestCtx.QueryArgs().Peek("name"))
	if strings.TrimSpace(name) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.Ts("globals.messages.required", "name", "name"), nil, envelope.InputError)
	}
	var body previewFunctionReq
	if err := r.Decode(&body, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if strings.TrimSpace(body.Code) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "code is required", nil, envelope.InputError)
	}
	lang := strings.ToLower(strings.TrimSpace(body.Language))
	if lang == "" {
		lang = "python"
	}

	// Build sdk.input from the {key, value} preview parameters.
	input := make(map[string]any, len(body.Parameters))
	for _, p := range body.Parameters {
		if p.Key != "" {
			input[p.Key] = p.Value
		}
	}

	// Use the saved function's timeout if one exists; otherwise the runner
	// default applies (TimeoutSeconds == 0).
	timeoutSec := 0
	if saved, err := app.function.GetByName(name); err == nil {
		timeoutSec = saved.TimeoutMS / 1000
		if timeoutSec <= 0 && saved.TimeoutMS > 0 {
			timeoutSec = 1
		}
	}

	result, err := app.execRunner.Execute(r.RequestCtx, execModels.Request{
		InvokerUserID:  user.ID,
		Code:           body.Code,
		Language:       lang,
		Packages:       function.SplitPackages(body.Packages),
		TimeoutSeconds: timeoutSec,
		Input:          input,
		ToolName:       "function-preview:" + name,
	})
	if err != nil {
		var runtimeErr *execrunner.RuntimeError
		if errors.As(err, &runtimeErr) {
			return r.SendErrorEnvelope(
				fasthttp.StatusInternalServerError,
				nonEmpty(runtimeErr.Message, "function runtime failed"),
				map[string]any{
					"kind":      "runtime_error",
					"code":      runtimeErr.Code,
					"message":   runtimeErr.Message,
					"traceback": runtimeErr.Traceback,
				},
				envelope.GeneralError,
			)
		}
		if status, msg, etype, handled := aitools.MapDispatchErrorToHTTP(err); handled {
			return r.SendErrorEnvelope(status, msg, nil, etype)
		}
		app.lo.Error("function preview failed", "name", name, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(result)
}
