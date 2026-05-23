// Tool dispatcher — single dispatch path for both kinds of AI tool:
//
//   - "custom" → ai_tools row (Python script, dispatched through the F6
//     sandbox).
//   - "internal" → builtin.InternalCapability (in-process Go function).
//
// Both paths run through the same parameter validator (internal/ai/tools/validate)
// so manual-invoke and the agent runtime see identical error messages on
// malformed input. Both paths write an audit row to tool_executions via
// the F6 runner's audit machinery (custom tools through the runner's
// Execute path; internal capabilities through dispatchInternal which
// builds a Result envelope and writes the same row shape).
//
// Callers pass parameters as a flat map (see DispatchRequest). Parameter
// sourcing from conversation/customer/contract context is the agent
// runtime's job, not the dispatcher's.
package aitools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentmodels "github.com/abhinavxd/libredesk/internal/ai/agents/models"
	"github.com/abhinavxd/libredesk/internal/ai/tools/builtin"
	"github.com/abhinavxd/libredesk/internal/ai/tools/models"
	"github.com/abhinavxd/libredesk/internal/ai/tools/validate"
	execRunner "github.com/abhinavxd/libredesk/internal/exec/runner"
	execModels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
)

// DispatchRequest is the input to Dispatch — the manual-invocation API
// caller's body normalized + the resolved invoker user.
type DispatchRequest struct {
	// Kind identifies which dispatch path to take. When zero-valued,
	// Dispatch defaults to "custom" (Python tool via the F6 sandbox) so
	// existing F9 callers keep working without changes.
	Kind agentmodels.ToolRefKind

	// Tool identifier. Semantics depend on Kind:
	//   - custom (default): Slug (preferred) or ID identifies an
	//     ai_tools row.
	//   - internal: Slug holds the InternalCapability ID (kb_search,
	//     get_customer_data, ...). ID is unused.
	Slug string
	ID   int

	// Caller identity for audit + sandbox callbacks.
	InvokerUserID int

	// LegitimationLevel is the caller's effective level for the
	// min_legitimation gate. Manual-invocation v1 takes this from an
	// optional body field defaulting to 0; the agent runtime will source
	// it from the conversation's legitimation state.
	LegitimationLevel int

	// Parameters maps directly to req.Input — what the SDK exposes as
	// sdk.input for custom tools and the params map for internal
	// capabilities. The dispatcher validates the shape against the
	// tool's declared input_parameters before invoking.
	Parameters map[string]any

	// ConversationID, when non-zero, is plumbed to the runner so the
	// SDK's sdk.api callbacks get default-scoping (custom tools), and
	// populated into the ToolRuntimeContext for internal capabilities.
	// Not used to source parameters — that's the agent runtime's job.
	ConversationID int

	// RuntimeCtx is supplied by callers invoking internal capabilities
	// that need manager handles (knowledge / conversation / user). Nil
	// is acceptable for capabilities that don't read managers and for
	// custom-kind requests. For convenience the dispatcher fills in
	// ConversationID / LegitimationLevel from the request fields when
	// they're zero on the supplied context.
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
	ErrToolNotFound      = errors.New("dispatcher: tool not found")
	ErrToolDisabled      = errors.New("dispatcher: tool disabled")
	ErrInsufficientLegit = errors.New("dispatcher: caller legitimation below tool's min_legitimation")
	ErrParamsInvalid     = errors.New("dispatcher: parameters failed schema validation")
	ErrImplInvalid       = errors.New("dispatcher: tool implementation is malformed")
	ErrCapabilityFailed  = errors.New("dispatcher: internal capability returned an error")
)

// runnerExecutor is the slice of execRunner.Manager the dispatcher needs.
// Defined as an interface so callers can substitute fakes in tests.
//
// RecordInternalAudit writes a tool_executions row for in-process
// internal-capability calls (custom tools get their audit row via
// Execute itself). Best-effort — failures are logged at the runner level.
type runnerExecutor interface {
	Execute(ctx context.Context, req execModels.Request) (*execModels.Result, error)
	RecordInternalAudit(ctx context.Context, row execModels.InternalAuditRow)
}

// Dispatcher executes both kinds of tool invocation through one path.
// Custom tools go through the F6 runner; internal capabilities run
// in-process via builtin.Default(). Both write an audit row to
// tool_executions with the same shape so ops gets a unified trail.
type Dispatcher struct {
	tools    *Manager
	runner   runnerExecutor
	registry *builtin.Registry
}

// NewDispatcher wires up the dispatcher. The internal-capability
// registry defaults to builtin.Default() when not supplied; tests pass
// an isolated registry to avoid global-state pollution.
func NewDispatcher(tools *Manager, runner runnerExecutor) *Dispatcher {
	return &Dispatcher{tools: tools, runner: runner, registry: builtin.Default()}
}

// NewDispatcherWithRegistry is the test-friendly constructor.
func NewDispatcherWithRegistry(tools *Manager, runner runnerExecutor, registry *builtin.Registry) *Dispatcher {
	if registry == nil {
		registry = builtin.Default()
	}
	return &Dispatcher{tools: tools, runner: runner, registry: registry}
}

// Dispatch resolves the tool, validates the request, and invokes through
// the right path (F6 sandbox for custom, in-process for internal).
// Returns an execModels.Result envelope in both cases so callers don't
// branch on kind. Non-dispatch failures (not found, disabled, validation,
// legitimation) come back as one of the Err* sentinels.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	// Default Kind for back-compat with F9 callers that didn't carry it.
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

// dispatchCustom is the F6-sandbox path — same flow as the F9 dispatcher,
// now using the shared validator.
func (d *Dispatcher) dispatchCustom(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	tool, err := d.loadTool(req)
	if err != nil {
		return nil, err
	}
	if !tool.Enabled {
		return nil, ErrToolDisabled
	}
	if req.LegitimationLevel < tool.MinLegitimation {
		return nil, ErrInsufficientLegit
	}
	if err := validate.Schema(tool.InputParameters, req.Parameters); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrParamsInvalid, err.Error())
	}

	var impl models.PythonScriptImpl
	if err := json.Unmarshal(tool.Implementation, &impl); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrImplInvalid, err.Error())
	}
	if strings.TrimSpace(impl.Code) == "" {
		return nil, fmt.Errorf("%w: implementation.code is empty", ErrImplInvalid)
	}

	timeoutSec := tool.TimeoutMS / 1000
	if timeoutSec <= 0 && tool.TimeoutMS > 0 {
		timeoutSec = 1
	}

	// Language is locked to "python" or "javascript" at save time (see
	// cmd/ai_tools.go validateAITool). Empty / legacy rows (created
	// before the lock) default to python — that's the safe choice since
	// every legacy row used the python SDK preamble.
	lang := strings.ToLower(strings.TrimSpace(impl.Language))
	if lang != "python" && lang != "javascript" {
		lang = "python"
	}

	rReq := execModels.Request{
		InvokerUserID:  req.InvokerUserID,
		Code:           impl.Code,
		Language:       lang,
		Packages:       splitPackages(impl.Packages),
		TimeoutSeconds: timeoutSec,
		Input:          req.Parameters,
		ToolName:       tool.Slug,
		ToolDepth:      0,
	}
	if req.ConversationID > 0 {
		rReq.ConversationID = &req.ConversationID
	}
	return d.runner.Execute(ctx, rReq)
}

// dispatchInternal is the in-process path — runs the InternalCapability,
// builds an execModels.Result envelope so callers see the same shape as
// custom-tool calls, and writes a tool_executions audit row through the
// runner's audit machinery.
//
// Parallel runner dispatch reuses one ToolRuntimeContext across same-turn
// read calls. To preserve that shared-state invariant, this method clones
// req.RuntimeCtx before filling request-local defaults like ConversationID
// and LegitimationLevel; it must never mutate the caller's shared pointer.
func (d *Dispatcher) dispatchInternal(ctx context.Context, req DispatchRequest) (*execModels.Result, error) {
	if d.registry == nil {
		d.registry = builtin.Default()
	}
	cap, found := d.registry.Get(req.Slug)
	if !found {
		return nil, ErrToolNotFound
	}
	if req.LegitimationLevel < cap.MinLegitimation() {
		return nil, ErrInsufficientLegit
	}

	// Run the capability's input schema (a JSON Schema) through the
	// shared validator. The validator was authored for the F5 array-of-
	// {key, type, required} shape; capabilities use plain JSON Schema.
	// validateInternalSchema bridges the two so error messages stay
	// consistent.
	if err := validateInternalSchema(cap.InputSchema(), req.Parameters); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrParamsInvalid, err.Error())
	}

	rt := cloneRuntimeContext(req.RuntimeCtx)
	if rt.ConversationID == 0 {
		rt.ConversationID = req.ConversationID
	}
	if rt.LegitimationLevel == 0 {
		rt.LegitimationLevel = req.LegitimationLevel
	}

	startedAt := time.Now()
	out, capErr := cap.Execute(ctx, req.Parameters, rt)
	durationMs := int(time.Since(startedAt).Milliseconds())

	executionID := newInternalExecutionID()
	if capErr != nil {
		// Surface the underlying error as ErrCapabilityFailed for the
		// HTTP handler's mapping; preserve the message for ops.
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

	// Wrap successful output in the runner's Result envelope so the HTTP
	// handler's response shape stays uniform across kinds. Result.Data is
	// typed map[string]any to match the sandbox's wire shape — round-trip
	// the capability's output through JSON to coerce.
	dataJSON, _ := json.Marshal(out)
	dataMap := map[string]any{}
	_ = json.Unmarshal(dataJSON, &dataMap)
	result := &execModels.Result{
		ExecutionID: executionID,
		Kind:        execModels.KindSuccess,
		Code:        execModels.CodeExecutionSuccess,
		Data:        dataMap,
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

// cloneRuntimeContext makes a shallow per-dispatch copy so callers may
// share a single ToolRuntimeContext across parallel tool executions
// without the dispatcher mutating their shared pointer when applying
// request-local defaults.
func cloneRuntimeContext(rt *builtin.ToolRuntimeContext) *builtin.ToolRuntimeContext {
	if rt == nil {
		return &builtin.ToolRuntimeContext{}
	}
	cp := *rt
	return &cp
}

// newInternalExecutionID mirrors the F6 runner's exec id format so audit
// rows look uniform regardless of how the tool ran.
func newInternalExecutionID() string {
	// Format: "exec_<16 hex>" — same shape as the runner emits.
	// We use crypto/rand via a tiny inline helper to avoid a new dep.
	b := make([]byte, 8)
	if _, err := randRead(b); err != nil {
		// Fallback to time-based — collision is acceptable here, this
		// is operator-facing audit, not security state.
		now := time.Now().UnixNano()
		return fmt.Sprintf("exec_%016x", now)
	}
	return fmt.Sprintf("exec_%x", b)
}

// randRead is var so tests can stub it deterministically.
var randRead = rand.Read

// validateInternalSchema bridges plain JSON Schema (what internal
// capabilities declare) to the same kind of validation the F5 array
// schema gets. We only check required-ness here; deep type checking is
// the capability's responsibility (typed Go code with native errors
// surfaces better failure messages than JSON Schema's enum-of-strings).
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
		// Empty strings on a required field are also invalid — they
		// usually mean the caller forgot to fill the form.
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
	// No GetBySlug on the manager yet; list-by-enabled-and-search isn't
	// the right shape either. Inline a single-row lookup here.
	t, err := d.tools.getBySlug(req.Slug)
	if err != nil {
		return t, ErrToolNotFound
	}
	return t, nil
}

// splitPackages breaks impl.Packages (a string holding pip requirements,
// possibly newline- or whitespace-separated) into a slice the runner accepts.
// Empty strings are dropped. Mirrors how Enneo authors typically wrote it.
func splitPackages(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ','
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// IsRunnerErrSandboxUnavailable / IsRunnerErrInvalidRequest are exposed so
// the HTTP handler can map runner errors to HTTP codes without importing
// the runner package directly. Keeps the handler dependency surface small.
func IsRunnerErrSandboxUnavailable(err error) bool {
	return errors.Is(err, execRunner.ErrSandboxUnavailable)
}
func IsRunnerErrInvalidRequest(err error) bool {
	return errors.Is(err, execRunner.ErrInvalidRequest)
}
