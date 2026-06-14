package airuns

import (
	"testing"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	"github.com/aziz-tb/merio/internal/ai/runner"
	"github.com/aziz-tb/merio/internal/composer"
	"github.com/stretchr/testify/require"
)

// TestComposerPromptFallback_MatchesBuilderFloor is the drift guard: the runner's
// minimal fallback (skeleton+decision floor, used for composer runs not built by
// the activity) must be byte-identical to the pure builder's empty-guidance,
// empty-roster output. This pins the two construction paths so the test-path
// floor and the prod-path builder cannot diverge in structure.
func TestComposerPromptFallback_MatchesBuilderFloor(t *testing.T) {
	dec := runner.ComposerDecisionInstructions()
	require.Equal(t,
		runner.MinimalComposerSystemPrompt(dec),
		composer.BuildComposerSystemPrompt(runner.ComposerSkeleton, dec, "", composer.Assembled{}),
	)
}

// TestFormatRoster proves the roster body: sorted by id, description omitted when
// blank, and "" when no specialists exist (the section is then omitted upstream).
func TestFormatRoster(t *testing.T) {
	require.Empty(t, formatRoster(nil))

	out := formatRoster([]agentmodels.Agent{
		{ID: 2, Slug: "cancellation", Description: "contracts"},
		{ID: 1, Slug: "billing", Description: "invoices"},
		{ID: 3, Slug: "meter"}, // no description
	})
	require.Equal(t, "- billing — invoices\n- cancellation — contracts\n- meter", out)
}
