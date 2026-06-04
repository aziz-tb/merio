package translator

import (
	"encoding/json"
	"testing"

	agentmodels "github.com/aziz-tb/merio/internal/ai/agents/models"
	toolmodels "github.com/aziz-tb/merio/internal/ai/tools/models"
	"github.com/aziz-tb/merio/internal/ai/tools/resolver"
	"github.com/stretchr/testify/require"
)

func TestTranslateCustomIncludesParamsWithLegacySourceFields(t *testing.T) {
	defs, err := Translate([]resolver.ResolvedTool{{
		Kind: agentmodels.ToolRefKindCustom,
		Custom: &toolmodels.Tool{
			Name: "Lookup",
			Slug: "lookup",
			InputParameters: json.RawMessage(`[
				{"key":"q","type":"string","required":true},
				{"key":"contract_id","type":"string","required":true,"source":"contract_context","source_key":"contract_id"}
			]`),
		},
	}})

	require.NoError(t, err)
	require.Len(t, defs, 1)
	props := defs[0].InputSchema["properties"].(map[string]any)
	require.Contains(t, props, "q")
	require.Contains(t, props, "contract_id")
	require.ElementsMatch(t, []string{"q", "contract_id"}, defs[0].InputSchema["required"])
}
