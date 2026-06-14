package airuns

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aziz-tb/merio/internal/ai/llm"
	"github.com/stretchr/testify/require"
)

func TestIsTransientProviderError(t *testing.T) {
	// Explicitly-emitted transient sentinels (+ forward-compat ErrTimeout).
	require.True(t, isTransientProviderError(llm.ErrRateLimited))
	require.True(t, isTransientProviderError(llm.ErrCircuitOpen))
	require.True(t, isTransientProviderError(llm.ErrTimeout))
	require.True(t, isTransientProviderError(context.DeadlineExceeded))
	// A wrapped transient is still detected (errors.Is unwraps).
	require.True(t, isTransientProviderError(fmt.Errorf("provider call: %w", llm.ErrRateLimited)))

	// Non-transient / bug / config / unknown -> false (fail-loud, the safe
	// direction). Raw 5xx + network errors arrive as non-sentinel errors and
	// are represented here by a plain error.
	require.False(t, isTransientProviderError(llm.ErrAuth))
	require.False(t, isTransientProviderError(llm.ErrInvalidRequest))
	require.False(t, isTransientProviderError(context.Canceled))
	require.False(t, isTransientProviderError(errors.New("raw 5xx / network error")))
	require.False(t, isTransientProviderError(nil))
}

func TestComposerErrorDisposition(t *testing.T) {
	transient := llm.ErrRateLimited
	bug := llm.ErrInvalidRequest
	unknown := errors.New("mystery")

	tests := []struct {
		name       string
		err        error
		isComposer bool
		willRetry  bool
		want       errorDisposition
	}{
		{"non-composer always fails loud", transient, false, false, dispositionFailLoud},
		{"nil error fails loud", nil, true, false, dispositionFailLoud},
		{"transient with a retry pending -> retry (don't cannibalize)", transient, true, true, dispositionRetry},
		{"transient, retries exhausted -> escalate", transient, true, false, dispositionEscalate},
		{"bug/config, retries exhausted -> fail loud", bug, true, false, dispositionFailLoud},
		{"unknown error, retries exhausted -> fail loud", unknown, true, false, dispositionFailLoud},
		{"bug with a retry pending -> retry (defer to Temporal)", bug, true, true, dispositionRetry},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, composerErrorDisposition(tc.err, tc.isComposer, tc.willRetry))
		})
	}
}
