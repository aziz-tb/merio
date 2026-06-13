// Tool dispatcher — single dispatch path for both kinds of AI tool:
// "custom" (ai_tools row, Python via the sandbox) and "internal"
// (builtin.InternalCapability, in-process Go). Both run through the same
// parameter validator and both write a function_executions audit row, so
// callers get identical error messages and a unified trail regardless of kind.

package aitools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/ai/tools/models"
	"github.com/aziz-tb/merio/internal/ai/tools/validate"
	execRunner "github.com/aziz-tb/merio/internal/exec/runner"
	execModels "github.com/aziz-tb/merio/internal/exec/runner/models"
	"github.com/aziz-tb/merio/internal/function"
)

// DispatchRequest is the input to Dispatch.
type DispatchRequest struct {
	// Kind selects the dispatch path; zero-valued defaults to "custom".
	Kind agentmodels.ToolRefKind

	// Tool identifier. For custom, Slug (preferred) or ID names an
	// ai_tools row; for internal, Slug is the InternalCapability ID
	// (kb_search, ...) and ID is unused.
	Slug string
	ID   int

	// Caller identity for audit + sandbox callbacks.
	InvokerUserID int

	// VerificationLevel is the caller's effective level, forwarded to
	// internal capabilities for their data-shaping decisions.
	VerificationLevel int

	// Parameters maps directly to req.Input; validated against the
	// tool's declared input_parameters before invoking.
	Parameters map[string]any

	// ConversationID, when non-zero, is plumbed to the runner for
	// sdk.api default-scoping, into ToolRuntimeContext for internal
	// capabilities, and as _CONVERSATION_UUID for sdk.context.* helpers.
	ConversationID int

	// RuntimeCtx supplies manager handles for internal capabilities that
	// read them; nil is fine otherwise. The dispatcher fills in
	// ConversationID / VerificationLevel from the request when zero here.
	RuntimeCtx *builtin.ToolRuntimeContext
}

// DispatchResult is the dispatcher's outcome. Either Result (sandbox ran)
// or Err (validation/dispatch failure) is set; never both.
type DispatchResult struct {
	Result *execModels.Result
	Err    error
}

// Sentinel errors so the HTTP handler can map to the right status code.
var (
	ErrToolNotFound                  = errors.New("dispatcher: tool not found")
	ErrToolDisabled                  = errors.New("dispatcher: tool disabled")
	ErrAgentVerificationInsufficient = errors.New("dispatcher: conversation verification below agent requirement")
	ErrParamsInvalid                 = errors.New("dispatcher: parameters failed schema validation")
	ErrImplInvalid                   = errors.New("dispatcher: tool implementation is malformed")
	ErrCapabilityFailed              = errors.New("dispatcher: internal capability returned an error")
)

// runnerExecutor is the slice of execRunner.Manager the dispatcher needs,
// as an interface so tests can substitute fakes. RecordInternalAudit writes
// the function_executions row for internal-capability calls (custom tools
// get theirs via Execute); best-effort, failures logged at the runner level.
type runnerExecutor interface {
	Execute(ctx context.Context, req execModels.Request) (*execModels.Result, error)
	RecordInternalAudit(ctx context.Context, row execModels.InternalAuditRow)
}

// functionRunner is the slice of function.Manager the dispatcher needs to
// run a custom tool's code; an interface so tests can substitute a fake.
type functionRunner interface {
	Run(ctx context.Context, req function.RunRequest) (*execModels.Result, error)
}

// Dispatcher executes both kinds of tool invocation through one path:
// custom tools via the function runner, internal capabilities in-process.
type Dispatcher struct {
	tools     *Manager
	runner    runnerExecutor
	functions functionRunner
	registry  *builtin.Registry
}

// NewDispatcher wires up the dispatcher with the default builtin registry.
func NewDispatcher(tools *Manager, runner runnerExecutor, functions functionRunner) *Dispatcher {
	return &Dispatcher{tools: tools, runner: runner, functions: functions, registry: builtin.Default()}
}

// NewDispatcherWithRegistry is the test-friendly constructor.
func NewDispatcherWithRegistry(tools *Manager, runner runnerExecutor, functions functionRunner, registry *builtin.Registry) *Dispatcher {
	if registry == nil {
		registry = builtin.Default()
	}
	return &Dispatcher{tools: tools, runner: runner, functions: functions, registry: registry}
}

// Dispatch resolves the tool, validates the request, and invokes. Returns
// an execModels.Result envelope for both kinds so callers don't branch;
// non-dispatch failures come back as one of the Err* sentinels.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	if req.Kind == "" {
		req.Kind = agentmodels.ToolRefKindCustom
	}
	switch req.Kind {
	case agentmodels.ToolRefKindCustom:
		return d.dispatchCustom(ctx, req)
	case agentmodels.ToolRefKindInternal:
		return d.dispatchInternal(ctx, req)
	default:
		return nil, fmt.Errorf("%w: unknown kind %q", ErrToolNotFound, req.Kind)
	}
}

// dispatchCustom is the sandbox path for Python custom tools.
func (d *Dispatcher) dispatchCustom(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	tool, err := d.loadTool(req)
	if err != nil {
		return nil, err
	}
	if !tool.Enabled {
		return nil, ErrToolDisabled
	}
	if err := validate.Schema(tool.InputParameters, req.Parameters); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrParamsInvalid, err.Error())
	}

	// Code lives on the linked functions row; run it by the tool's slug
	// (== function name) so dispatch and the customer-identification
	// workflow share one path. The runner writes the audit row.
	result, err := d.functions.Run(ctx, function.RunRequest{
		Name:           tool.Slug,
		Input:          req.Parameters,
		InvokerUserID:  req.InvokerUserID,
		ConversationID: req.ConversationID,
	})
	if err != nil {
		return nil, translateFunctionErr(err)
	}
	return result, nil
}

// translateFunctionErr maps function-package sentinels onto dispatcher
// sentinels so MapDispatchErrorToHTTP yields the right status codes. Infra
// errors (sandbox unavailable, invalid request) pass through untouched.
func translateFunctionErr(err error) error {
	switch {
	case errors.Is(err, function.ErrFunctionNotFound):
		return fmt.Errorf("%w: linked function not found", ErrImplInvalid)
	case errors.Is(err, function.ErrFunctionCodeEmpty):
		return fmt.Errorf("%w: function code is empty", ErrImplInvalid)
	default:
		return err
	}
}

// dispatchInternal runs an InternalCapability in-process, wraps its output
// in an execModels.Result envelope, and writes the audit row.
//
// Callers may share one ToolRuntimeContext across parallel same-turn calls,
// so this clones req.RuntimeCtx before filling request-local defaults; it
// must never mutate the caller's shared pointer.
func (d *Dispatcher) dispatchInternal(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	if d.registry == nil {
		d.registry = builtin.Default()
	}
	cap, found := d.registry.Get(req.Slug)
	if !found {
		return nil, ErrToolNotFound
	}

	// Capabilities declare plain JSON Schema; validateInternalSchema
	// bridges it to the shared validator so error messages stay consistent.
	if err := validateInternalSchema(cap.InputSchema(), req.Parameters); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrParamsInvalid, err.Error())
	}

	rt := cloneRuntimeContext(req.RuntimeCtx)
	if rt.ConversationID == 0 {
		rt.ConversationID = req.ConversationID
	}
	if rt.VerificationLevel == 0 {
		rt.VerificationLevel = req.VerificationLevel
	}

	startedAt := time.Now()
	out, capErr := cap.Execute(ctx, req.Parameters, rt)
	durationMs := int(time.Since(startedAt).Milliseconds())

	executionID := newInternalExecutionID()
	if capErr != nil {
		// Surface as ErrCapabilityFailed for the HTTP mapping; keep the message.
		errMsg := capErr.Error()
		if d.runner != nil {
			d.runner.RecordInternalAudit(ctx, execModels.InternalAuditRow{
				ExecutionID:    executionID,
				ToolName:       req.Slug,
				InvokerUserID:  req.InvokerUserID,
				ConversationID: req.ConversationID,
				StartedAt:      startedAt,
				DurationMs:     durationMs,
				Kind:           "error",
				Message:        errMsg,
				TraceID:        rt.TraceID,
			})
		}
		return nil, fmt.Errorf("%w: %s", ErrCapabilityFailed, errMsg)
	}

	// Wrap output in the Result envelope so the response shape stays
	// uniform across kinds; round-trip through JSON to coerce.
	dataJSON, _ := json.Marshal(out)
	result := &execModels.Result{
		ExecutionID: executionID,
		Kind:        execModels.KindSuccess,
		Code:        execModels.CodeExecutionSuccess,
		Data:        dataJSON,
		StartedAt:   startedAt,
		DurationMs:  durationMs,
	}
	if d.runner != nil {
		d.runner.RecordInternalAudit(ctx, execModels.InternalAuditRow{
			ExecutionID:    executionID,
			ToolName:       req.Slug,
			InvokerUserID:  req.InvokerUserID,
			ConversationID: req.ConversationID,
			StartedAt:      startedAt,
			DurationMs:     durationMs,
			Kind:           "success",
			Data:           dataJSON,
			TraceID:        rt.TraceID,
		})
	}
	return result, nil
}

// cloneRuntimeContext makes a shallow per-dispatch copy so applying
// request-local defaults never mutates a caller's shared pointer.
func cloneRuntimeContext(rt *builtin.ToolRuntimeContext) *builtin.ToolRuntimeContext {
	if rt == nil {
		return &builtin.ToolRuntimeContext{}
	}
	cp := *rt
	return &cp
}

// newInternalExecutionID mirrors the runner's "exec_<16 hex>" id format so
// audit rows look uniform regardless of how the tool ran.
func newInternalExecutionID() string {
	b := make([]byte, 8)
	if _, err := randRead(b); err != nil {
		// Fallback to time-based — collision is acceptable for audit.
		now := time.Now().UnixNano()
		return fmt.Sprintf("exec_%016x", now)
	}
	return fmt.Sprintf("exec_%x", b)
}

// randRead is var so tests can stub it deterministically.
var randRead = rand.Read

// validateInternalSchema checks required-ness on a capability's plain JSON
// Schema. Deep type checking is left to the capability's typed Go code,
// which surfaces better failure messages.
func validateInternalSchema(schema map[string]any, params map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	required, _ := schema["required"].([]string)
	if required == nil {
		// JSON-decoded schemas often hold required as []any.
		if anyArr, ok := schema["required"].([]any); ok {
			for _, e := range anyArr {
				if s, ok := e.(string); ok {
					required = append(required, s)
				}
			}
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	for _, k := range required {
		v, present := params[k]
		if !present || v == nil {
			return fmt.Errorf("required parameter %q is missing", k)
		}
		// Empty strings on a required field count as missing.
		if s, ok := v.(string); ok && s == "" {
			return fmt.Errorf("required parameter %q is missing", k)
		}
	}
	return nil
}

// loadTool fetches by slug or by ID, returning a typed not-found error
// rather than the generic envelope error from the manager.
func (d *Dispatcher) loadTool(req DispatchRequest) (models.Tool, error) {
	if req.ID > 0 {
		t, err := d.tools.Get(req.ID)
		if err != nil {
			return t, ErrToolNotFound
		}
		return t, nil
	}
	if req.Slug == "" {
		return models.Tool{}, ErrToolNotFound
	}
	t, err := d.tools.getBySlug(req.Slug)
	if err != nil {
		return t, ErrToolNotFound
	}
	return t, nil
}

// IsRunnerErrSandboxUnavailable / IsRunnerErrInvalidRequest let the HTTP
// handler map runner errors without importing the runner package directly.
func IsRunnerErrSandboxUnavailable(err error) bool {
	return errors.Is(err, execRunner.ErrSandboxUnavailable)
}

// IsRunnerErrInvalidRequest reports whether err is the runner's
// invalid-request sentinel — the handler maps it to 400.
func IsRunnerErrInvalidRequest(err error) bool {
	return errors.Is(err, execRunner.ErrInvalidRequest)
}
