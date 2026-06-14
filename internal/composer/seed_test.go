package composer

import (
	"testing"

	"github.com/aziz-tb/merio/internal/composer/models"
	cmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/stretchr/testify/require"
)

// defaultVerificationGuidance mirrors the default guidance item seeded into
// composer_config by schema.sql and migration V0_9_0. This test is the safety
// net for that hand-authored seed JSON: if either drifts into an invalid shape,
// this fails. Keep the three in sync.
const defaultVerificationGuidance = `[{"id":"verification-default","title":"Ask unverified customers to verify before account actions","group":"other","body_text":"If the customer is not verified and asks about or requests an action on their own account (billing, contract, personal data, or account changes), ask them to complete identity verification before acting on it. General questions — opening hours, how-to, product info — do not require verification.","inbox_scope":[],"conditions":{"op":"AND","children":[{"leaf":{"source":"ticket","path":"verification_level","operator":"<","value":2}}]},"status":"live"}]`

// TestDefaultVerificationSeed proves the seeded default verification guidance
// item is a valid, well-formed config item AND behaves: it decodes strictly,
// passes validation, applies to an unverified ticket, and stops applying once
// the customer is verified.
func TestDefaultVerificationSeed(t *testing.T) {
	raw := `{"guidance":` + defaultVerificationGuidance + `,"automation":{"auto_send":false,"approval_required_tool_slugs":[]},"escalation":{"rules":[],"guidance":[]},"capability":{"kb_scope":{"folder_ids":[],"article_ids":[]},"tool_slugs":[]}}`

	// Strict decode (catches shape/unknown-field errors that would break GetConfig).
	cfg, err := DecodeStrict([]byte(raw))
	require.NoError(t, err)
	require.Len(t, cfg.Guidance, 1)
	item := cfg.Guidance[0]
	require.Equal(t, models.GroupOther, item.Group)
	require.Equal(t, models.StatusLive, item.Status)
	require.NotNil(t, item.Conditions)

	// Semantic validation passes (legal group/status/condition source).
	refs := ValidationRefs{AllowedSources: []string{
		detection.SourceTicket, detection.SourceCustomer, detection.SourceContract, detection.SourceTags,
	}}
	require.NoError(t, ValidateConfig(cfg, refs))

	// Behaves: applies while unverified (level 0), not once verified (level 2).
	eff := Resolve(cfg, 0)
	unver := Assemble(eff, detection.EvaluationContext{Conversation: cmodels.Conversation{VerificationLevel: 0}})
	require.Len(t, unver.Guidance, 1, "default item applies to an unverified ticket")
	ver := Assemble(eff, detection.EvaluationContext{Conversation: cmodels.Conversation{VerificationLevel: 2}})
	require.Empty(t, ver.Guidance, "default item stops applying once verified")
}
