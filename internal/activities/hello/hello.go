// Package hello holds the smoke-test activity that verifies a Temporal
// worker is wired up end-to-end. Not for production workflow logic.
package hello

import (
	"context"
	"fmt"

	activitydeps "github.com/aziz-tb/merio/internal/activities/dependencies"
)

// SayHelloInput is the activity input. Pointer-to-struct is the
// project-wide activity convention; see internal/temporal/CONVENTIONS.md.
type SayHelloInput struct {
	Name string
}

// SayHelloOutput is the activity output.
type SayHelloOutput struct {
	Greeting string
}

// Activities groups the smoke-test activities for dependency injection.
// Currently no dependencies.
type Activities struct{}

// New returns an Activities ready to be registered with a worker.
func New() *Activities {
	return &Activities{}
}

func NewActivities(deps activitydeps.Dependencies) (*Activities, error) {
	_ = deps
	return New(), nil
}

// SayHello is the canonical smoke activity, used by the hello-world
// workflow and tests to verify the worker runs activity tasks. Returns
// an error if Name is empty.
func (a *Activities) SayHello(_ context.Context, in *SayHelloInput) (*SayHelloOutput, error) {
	if in == nil || in.Name == "" {
		return nil, fmt.Errorf("hello: Name is required")
	}
	return &SayHelloOutput{Greeting: "hello, " + in.Name}, nil
}
