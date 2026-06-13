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

// Client wraps go.temporal.io/sdk/client.Client. Start workflows via
// [Client.StartWorkflow] rather than the SDK so every start carries a
// deterministic ID + search attributes.
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
// Returns (nil, nil) when [Config.Enabled] is false; callers MUST nil-check
// the result (the graceful-degradation contract, see CONVENTIONS.md). Dial
// failures return a non-nil error but are not fatal — callers decide whether
// to retry (worker binary) or fall back to sync (main API binary).
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

	// Register custom search attributes. Short dedicated timeout so a slow
	// Temporal server can't block boot; failure is logged, non-fatal.
	saCtx, saCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saCancel()
	c.registerSearchAttributes(saCtx)

	return c, nil
}

// registerSearchAttributes ensures the custom search attributes exist on the
// namespace, all as KEYWORD. Idempotent per attribute; non-AlreadyExists
// errors are logged at warn and boot continues.
//
// One RPC per attribute, not a batch: the batched form fails atomically with
// AlreadyExists if ANY attribute already exists, leaving the rest
// unregistered. Per-attribute calls reconcile a partial namespace on every
// boot.
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

// SDK exposes the underlying Temporal client. Use sparingly — prefer the
// wrapper's StartWorkflow / SignalWorkflow; reach for SDK only where no
// wrapper exists yet (e.g. ListWorkflowExecutions).
func (c *Client) SDK() client.Client {
	return c.sdk
}

// Config returns a copy of the config the client was built with.
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

// StartOptions is the deliberately small subset of
// client.StartWorkflowOptions this codebase uses.
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
	// running but a completed one with this ID exists. Zero value = SDK default
	// (AllowDuplicate); other values are the WORKFLOW_ID_REUSE_POLICY_* enums
	// in go.temporal.io/api/enums/v1.
	ReusePolicy enumspb.WorkflowIdReusePolicy

	// ErrorWhenAlreadyStarted makes ExecuteWorkflow return an error when the ID
	// is already running (or ReusePolicy disallows reuse) instead of returning
	// the current/last WorkflowRun handle. Keep false unless the caller must
	// surface "already in progress" to the user.
	ErrorWhenAlreadyStarted bool

	// WorkflowExecutionTimeout is a coarse safety net for the entire
	// workflow execution.
	WorkflowExecutionTimeout time.Duration
}

// StartWorkflow asynchronously starts a workflow and returns its
// client.WorkflowRun handle; usage is normally fire-and-forget (calling
// .Get on the handle blocks for the result). Errors if the wrapper is nil,
// so a missed app.temporal == nil check fails loudly instead of nil-derefing.
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
