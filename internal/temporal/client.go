package temporal

import (
	"context"
	"errors"
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"github.com/zerodha/logf"
)

// ErrAlreadyStarted is returned when a deterministic workflow ID is already
// running and REJECT_DUPLICATE rejects a duplicate enqueue.
var ErrAlreadyStarted = errors.New("temporal: workflow already started")

// IsAlreadyStarted reports whether err is a REJECT_DUPLICATE already-started
// failure, including this package's sentinel wrapper.
func IsAlreadyStarted(err error) bool {
	if errors.Is(err, ErrAlreadyStarted) {
		return true
	}
	var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
	return errors.As(err, &alreadyStarted)
}

// Client wraps go.temporal.io/sdk/client.Client with project-level
// conventions. Workflow-starting code in handlers and managers reaches
// for [Client.StartWorkflow] instead of the underlying SDK to ensure
// every start carries deterministic ID + search attributes.
type Client struct {
	sdk client.Client
	lo  *logf.Logger
	cfg Config
}

// Opts is the constructor input.
type Opts struct {
	Config Config
	Lo     *logf.Logger
}

// New dials the Temporal frontend and returns a wrapped client.
//
// Returns nil + nil when [Config.Enabled] is false. Callers MUST
// check the returned *Client for nil before use — that is the
// graceful-degradation contract (see CONVENTIONS.md).
//
// Dial failures are NOT fatal: a Temporal outage shouldn't take down
// the inbox. Callers see a non-nil error and decide whether to retry
// (e.g., the worker binary) or to keep app.temporal nil and fall
// back to sync (e.g., the main API binary).
func New(opt Opts) (*Client, error) {
	if !opt.Config.Enabled {
		return nil, nil
	}
	if opt.Lo == nil {
		return nil, errors.New("temporal: Lo is required")
	}
	if opt.Config.Address == "" {
		return nil, errors.New("temporal: Address is required when enabled")
	}
	if opt.Config.Namespace == "" {
		return nil, errors.New("temporal: Namespace is required when enabled")
	}

	sdkClient, err := client.Dial(client.Options{
		HostPort:  opt.Config.Address,
		Namespace: opt.Config.Namespace,
		Logger:    NewSDKLogger(opt.Lo),
	})
	if err != nil {
		return nil, fmt.Errorf("temporal: dial %s: %w", opt.Config.Address, err)
	}

	opt.Lo.Info("temporal client connected",
		"address", opt.Config.Address,
		"namespace", opt.Config.Namespace)

	c := &Client{sdk: sdkClient, lo: opt.Lo, cfg: opt.Config}

	// Register the project's custom search attributes with the
	// namespace. Idempotent server-side: re-registering an existing
	// attribute returns AlreadyExists which we swallow. A short
	// dedicated timeout keeps a slow Temporal server from blocking
	// boot indefinitely; failure is logged but non-fatal — the client
	// stays usable, only filter-by-attribute in the UI breaks until
	// the next boot.
	saCtx, saCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saCancel()
	c.registerSearchAttributes(saCtx)

	return c, nil
}

// registerSearchAttributes ensures the project's custom search
// attributes (defined in search_attributes.go) exist on the namespace.
// All registered as KEYWORD so the OperatorService accepts the same
// shape regardless of caller-side type (numeric inbox_id / entity_id
// are stamped as their string form at workflow-start time).
//
// One RPC per attribute, not a single batched RPC. Rationale: the
// batched form fails atomically with AlreadyExists when ANY single
// attribute is already registered, leaving the remaining (missing)
// attributes unregistered. Per-attribute calls let each one succeed
// or skip independently so a partially-registered namespace gets
// reconciled on every boot. Five RPCs at boot is cheap.
//
// Idempotent per attribute. Other errors are logged at warn level and
// the boot continues — the client stays usable, only filter-by-that-
// attribute in the UI is degraded until the next boot.
func (c *Client) registerSearchAttributes(ctx context.Context) {
	attrs := []string{SATraceID, SAWorkflowKind, SAEntityType, SAEntityID, SAInboxID, SAAgentID}
	var registered, existing []string
	for _, name := range attrs {
		req := &operatorservice.AddSearchAttributesRequest{
			Namespace: c.cfg.Namespace,
			SearchAttributes: map[string]enumspb.IndexedValueType{
				name: enumspb.INDEXED_VALUE_TYPE_KEYWORD,
			},
		}
		_, err := c.sdk.OperatorService().AddSearchAttributes(ctx, req)
		if err == nil {
			registered = append(registered, name)
			continue
		}
		var alreadyExists *serviceerror.AlreadyExists
		if errors.As(err, &alreadyExists) {
			existing = append(existing, name)
			continue
		}
		c.lo.Warn("temporal search-attribute registration failed (filter-by-attribute may be unavailable until next boot)",
			"namespace", c.cfg.Namespace,
			"attribute", name,
			"error", err)
	}
	c.lo.Info("temporal search attributes synced",
		"namespace", c.cfg.Namespace,
		"newly_registered", registered,
		"already_existed", existing)
}

// SDK exposes the underlying Temporal client. Use sparingly — most
// call sites should go through StartWorkflow / SignalWorkflow on this
// wrapper so search-attribute attachment and logging stay consistent.
// SDK access is needed for things like ListWorkflowExecutions where
// no wrapper convenience exists yet.
func (c *Client) SDK() client.Client {
	return c.sdk
}

// Config returns a copy of the config the client was built with.
// Useful for the worker binary, which reads worker-tuning values from
// the same struct.
func (c *Client) Config() Config {
	return c.cfg
}

// Close releases the gRPC connection. Safe to call on a nil receiver
// (no-op) so the main app's shutdown sequence doesn't have to branch.
func (c *Client) Close() {
	if c == nil || c.sdk == nil {
		return
	}
	c.sdk.Close()
}

// StartOptions carries the per-start arguments. Mirrors the subset of
// client.StartWorkflowOptions we use consistently across the codebase
// — keeping this struct intentionally small forces every caller to
// think about the load-bearing fields.
type StartOptions struct {
	// ID is the workflow ID — MUST be deterministic from inputs.
	// See CONVENTIONS.md for the format.
	ID string

	// TaskQueue is one of the constants from queues.go.
	TaskQueue string

	// SearchAttributes attach at workflow start; see search_attributes.go
	// for the canonical names.
	SearchAttributes map[string]any

	// ReusePolicy controls server behaviour when no workflow with this ID is
	// currently running, but a completed workflow with this ID already exists.
	// Zero value = SDK default (AllowDuplicate). Common values from
	// go.temporal.io/api/enums/v1:
	//   - WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE
	//   - WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY
	//   - WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE
	//   - WORKFLOW_ID_REUSE_POLICY_TERMINATE_IF_RUNNING
	ReusePolicy enumspb.WorkflowIdReusePolicy

	// ErrorWhenAlreadyStarted makes ExecuteWorkflow return an error when the
	// workflow ID is already running, or when ReusePolicy disallows reusing a
	// completed workflow ID. When false, the SDK may return the current/last
	// WorkflowRun handle instead. Keep false unless the caller needs to surface
	// "already in progress" to the user.
	ErrorWhenAlreadyStarted bool

	// WorkflowExecutionTimeout is a coarse safety net for the entire
	// workflow execution.
	WorkflowExecutionTimeout time.Duration
}

// StartWorkflow asynchronously starts a workflow. Returns the
// client.WorkflowRun handle (fire-and-forget is the standard usage;
// callers rarely .Get(ctx, &result) the result, because that blocks).
//
// Returns an error if the wrapper is nil (caller should branch on
// app.temporal == nil before reaching here — but we guard anyway so
// a missed check fails loudly instead of nil-derefing).
func (c *Client) StartWorkflow(
	ctx context.Context,
	opts StartOptions,
	workflowFn any,
	args ...any,
) (client.WorkflowRun, error) {
	if c == nil {
		return nil, errors.New("temporal: client is nil (Temporal disabled?)")
	}
	if opts.ID == "" {
		return nil, errors.New("temporal: StartOptions.ID is required")
	}
	if opts.TaskQueue == "" {
		return nil, errors.New("temporal: StartOptions.TaskQueue is required")
	}

	wfOpts := client.StartWorkflowOptions{
		ID:                                       opts.ID,
		TaskQueue:                                opts.TaskQueue,
		SearchAttributes:                         opts.SearchAttributes,
		WorkflowIDReusePolicy:                    opts.ReusePolicy,
		WorkflowExecutionErrorWhenAlreadyStarted: opts.ErrorWhenAlreadyStarted,
		WorkflowExecutionTimeout:                 opts.WorkflowExecutionTimeout,
	}
	return c.sdk.ExecuteWorkflow(ctx, wfOpts, workflowFn, args...)
}
