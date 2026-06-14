package composer

import (
	"testing"

	"github.com/aziz-tb/merio/internal/composer/models"
	"github.com/aziz-tb/merio/internal/detection"
	"github.com/stretchr/testify/require"
)

func testRefs() ValidationRefs {
	return ValidationRefs{
		WriteToolSlugs: map[string]bool{"cancel_contract": true},
		InboxIDs:       map[int]bool{1: true, 2: true},
		AllowedSources: []string{
			detection.SourceTicket, detection.SourceCustomer,
			detection.SourceContract, detection.SourceTags,
		},
	}
}

func okItem(id string) models.GuidanceItem {
	return models.GuidanceItem{
		ID: id, Title: id, Group: models.GroupOther, BodyText: "x",
		InboxScope: []int{1}, Status: models.StatusLive,
	}
}

// TestDecodeStrict_NestedConfig confirms a valid config whose guidance item
// carries a real (nested) condition tree decodes cleanly under
// DisallowUnknownFields — the detection AST's own fields (op/children/tree/leaf/
// source/path/operator/value) are all known to the decoder and don't trip it.
func TestDecodeStrict_NestedConfig(t *testing.T) {
	raw := []byte(`{
		"guidance": [{
			"id": "g1",
			"title": "Sie tone",
			"group": "communication_style",
			"body_text": "Use formal Sie.",
			"inbox_scope": [1],
			"status": "live",
			"conditions": {
				"op": "AND",
				"children": [
					{"leaf": {"source": "ticket", "path": "subject", "operator": "contains", "value": "rechnung"}},
					{"tree": {"op": "OR", "children": [
						{"leaf": {"source": "tags", "path": "", "operator": "in", "value": ["vip"]}}
					]}}
				]
			}
		}],
		"automation": {"auto_send": true, "approval_required_tool_slugs": ["cancel_contract"]},
		"escalation": {
			"rules": [{"op": "AND", "children": [
				{"leaf": {"source": "tags", "path": "", "operator": "in", "value": ["legal"]}}
			]}],
			"guidance": []
		}
	}`)

	cfg, err := DecodeStrict(raw)
	require.NoError(t, err)
	require.Len(t, cfg.Guidance, 1)
	require.NotNil(t, cfg.Guidance[0].Conditions)
	require.Equal(t, detection.LogicalAnd, cfg.Guidance[0].Conditions.Op)
	require.Len(t, cfg.Guidance[0].Conditions.Children, 2)
	require.Len(t, cfg.Escalation.Rules, 1)
	require.True(t, cfg.Automation.AutoSend)

	// And the decoded config validates against matching refs.
	require.NoError(t, ValidateConfig(cfg, testRefs()))
}

func TestDecodeStrict_RejectsUnknownField(t *testing.T) {
	raw := []byte(`{"guidance": [{"id":"g1","group":"other","status":"live","surprise":true}]}`)
	_, err := DecodeStrict(raw)
	require.Error(t, err)
}

func TestValidateConfig(t *testing.T) {
	refs := testRefs()

	t.Run("clean config passes", func(t *testing.T) {
		cfg := models.Config{
			Guidance:   []models.GuidanceItem{okItem("g1")},
			Automation: models.Automation{ApprovalRequiredToolSlugs: []string{"cancel_contract"}},
			Escalation: models.Escalation{
				Rules: []detection.ConditionTree{{Op: detection.LogicalAnd, Children: []detection.ConditionNode{
					{Leaf: &detection.Condition{Source: detection.SourceTags, Operator: detection.OpIn, Value: "legal"}},
				}}},
				Guidance: []models.GuidanceItem{okItem("e1")},
			},
		}
		require.NoError(t, ValidateConfig(cfg, refs))
	})

	t.Run("unknown approval tool slug rejected", func(t *testing.T) {
		cfg := models.Config{Automation: models.Automation{ApprovalRequiredToolSlugs: []string{"ghost_tool"}}}
		err := ValidateConfig(cfg, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not an existing custom write tool")
	})

	t.Run("unknown inbox in scope rejected", func(t *testing.T) {
		item := okItem("g1")
		item.InboxScope = []int{99}
		err := ValidateConfig(models.Config{Guidance: []models.GuidanceItem{item}}, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nonexistent inbox id=99")
	})

	t.Run("invalid group rejected", func(t *testing.T) {
		item := okItem("g1")
		item.Group = "not_a_group"
		err := ValidateConfig(models.Config{Guidance: []models.GuidanceItem{item}}, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "group")
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		item := okItem("g1")
		item.Status = "archived"
		err := ValidateConfig(models.Config{Guidance: []models.GuidanceItem{item}}, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "status")
	})

	t.Run("invalid condition source rejected", func(t *testing.T) {
		item := okItem("g1")
		item.Conditions = &detection.ConditionTree{Op: detection.LogicalAnd, Children: []detection.ConditionNode{
			{Leaf: &detection.Condition{Source: "inbox", Path: "id", Operator: detection.OpEquals, Value: 1}},
		}}
		err := ValidateConfig(models.Config{Guidance: []models.GuidanceItem{item}}, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid condition source")
	})

	t.Run("invalid escalation rule source rejected", func(t *testing.T) {
		cfg := models.Config{Escalation: models.Escalation{Rules: []detection.ConditionTree{
			{Op: detection.LogicalAnd, Children: []detection.ConditionNode{
				{Leaf: &detection.Condition{Source: "inbox", Path: "id", Operator: detection.OpEquals, Value: 1}},
			}},
		}}}
		err := ValidateConfig(cfg, refs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "escalation.rules[0]")
	})
}
