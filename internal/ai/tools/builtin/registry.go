package builtin

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// InternalCapability is the contract every Go-side capability implements.
// Each capability is a stateless metadata bundle plus an Execute function;
// state (DB connections, manager handles) flows in via ToolRuntimeContext.
//
// Capabilities are registered at startup through init() blocks in
// .../builtin/capabilities/*. The agent runtime resolves them through
// the Registry; nothing else accesses InternalCapability instances
// directly.
type InternalCapability interface {
	// ID returns the stable identifier the LLM and agent.tools array
	// reference (e.g., "kb_search"). Must match ^[a-z][a-z0-9_]*$ —
	// the unified dispatcher uses ID as the slug equivalent for
	// internal capabilities.
	ID() string

	// Name returns the display name shown in admin UIs.
	Name() string

	// Description is the LLM-facing string included with every tool
	// definition the model sees. Wording matters: it influences whether
	// the model decides to call the tool. Reviewed at chunk-8 time.
	Description() string

	// InputSchema returns a JSON Schema (subset: object type, basic
	// property types, enum, required) describing the parameters the
	// capability accepts. Translated to the provider's tool-call format
	// in chunk 11.
	InputSchema() map[string]any

	// OutputSchema returns an optional JSON Schema describing the
	// shape Execute returns. Surfaced to the LLM so it can plan
	// follow-up calls. Return nil when no schema applies.
	OutputSchema() map[string]any

	// Category returns the dispatch category for this capability.
	// Valid values: "read" or "write".
	//
	// Read capabilities are eligible for parallel dispatch within a
	// single agent turn. Write capabilities execute sequentially after
	// all reads complete. This affects only runtime dispatch — not LLM
	// behavior and not tool schema.
	Category() string

	// Execute runs the capability against the supplied parameters and
	// runtime context. Returns whatever output makes sense for the
	// capability — a struct, a map, a slice. The agent runtime
	// JSON-encodes the result for the LLM. On failure, return an error
	// (the dispatcher wraps it as an execution_error envelope).
	Execute(ctx context.Context, params map[string]any, rt *ToolRuntimeContext) (any, error)
}

// Registry is the in-memory map of capability ID → InternalCapability.
// Concurrency-safe — registration happens at startup (single goroutine)
// but Get/List can be called from any worker, so the mutex is cheap.
type Registry struct {
	mu   sync.RWMutex
	caps map[string]InternalCapability
}

// NewRegistry constructs an empty Registry. Tests use this directly to
// build isolated registries; production uses the package-level singleton
// reachable via RegisterInternal / Default().
func NewRegistry() *Registry {
	return &Registry{caps: map[string]InternalCapability{}}
}

// Register adds cap to the registry. Returns an error on duplicate IDs
// — capabilities are platform code, duplicate IDs are bugs (typo in a
// new capability or accidental copy-paste).
func (r *Registry) Register(cap InternalCapability) error {
	if cap == nil {
		return fmt.Errorf("builtin: cannot register nil capability")
	}
	id := cap.ID()
	if id == "" {
		return fmt.Errorf("builtin: capability has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.caps[id]; dup {
		return fmt.Errorf("builtin: duplicate capability ID %q", id)
	}
	r.caps[id] = cap
	return nil
}

// Get returns the capability with the given id, or nil + false. Used by
// the unified dispatcher (chunk 10) and the test endpoint.
func (r *Registry) Get(id string) (InternalCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.caps[id]
	return c, ok
}

// List returns all registered capabilities sorted by ID. Used by the
// agent edit page's tool picker and by ad-hoc diagnostics.
func (r *Registry) List() []InternalCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.caps))
	for id := range r.caps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]InternalCapability, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.caps[id])
	}
	return out
}

// IDs returns just the registered capability IDs sorted. Cheap helper
// for log lines and error messages that want to surface available IDs
// when a caller asks for an unknown one.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.caps))
	for id := range r.caps {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// defaultRegistry is the package-level singleton populated by init()
// blocks in .../builtin/capabilities/*. Production uses this; tests use
// NewRegistry() to avoid global state leaking across tests.
var defaultRegistry = NewRegistry()

// Default returns the singleton registry that init() blocks register
// against. The agent runtime (chunk 10/12) holds a reference to this
// at startup and never mutates it after.
func Default() *Registry { return defaultRegistry }

// RegisterInternal is the package-level register-into-singleton call
// that capability init() blocks use:
//
//	func init() {
//	    builtin.RegisterInternal(&kbSearch{})
//	}
//
// Panics on duplicate IDs because the failure mode is "the binary will
// not boot consistently anyway"; better to surface it loudly during a
// fresh startup than to swallow it.
func RegisterInternal(cap InternalCapability) {
	if err := defaultRegistry.Register(cap); err != nil {
		panic(err)
	}
}

// ResetDefault resets the package-level singleton. Tests use this to
// restore a clean state when they exercise registration paths that
// would otherwise pollute later tests.
func ResetDefault() { defaultRegistry = NewRegistry() }
