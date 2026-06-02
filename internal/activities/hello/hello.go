// Package hello holds the smoke-test activity used to verify a
// Temporal worker is wired up end-to-end. Not for production
// workflow logic — F12 (embeddings) and later F-pieces build domain
// activities under sibling packages.
package hello

import (
	"context"
	"fmt"

	activitydeps "github.com/abhinavxd/libredesk/internal/activities/dependencies"
)

// SayHelloInput is the activity input. Pointer-to-struct is the
// convention for all activities in the project — see
// internal/temporal/CONVENTIONS.md.
type SayHelloInput struct {
	Name string
}

// SayHelloOutput is the activity output.
type SayHelloOutput struct {
	Greeting string
}

// Activities groups the smoke-test activities for dependency
// injection. Currently no dependencies (the smoke test is
// intentionally minimal) — left in place so future smoke activities
// inherit the same shape.
type Activities struct{}

// New returns an Activities ready to be registered with a worker.
func New() *Activities {
	return &Activities{}
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	_ = deps
	return New(), nil
}

// SayHello is the canonical smoke activity. Used by the hello-world
// workflow + tests to verify the worker actually picks up and runs
// activity tasks.
func (a *Activities) SayHello(_ context.Context, in *SayHelloInput) (*SayHelloOutput, error) {
	if in == nil || in.Name == "" {
		return nil, fmt.Errorf("hello: Name is required")
	}
	return &SayHelloOutput{Greeting: "hello, " + in.Name}, nil
}
