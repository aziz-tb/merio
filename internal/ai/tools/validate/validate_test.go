package validate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaIgnoresUnknownSourceFieldsAndRequiresLLMParam(t *testing.T) {
	schema := json.RawMessage(`[
		{"key":"q","type":"string","required":true},
		{"key":"contract_id","type":"string","required":true,"source":"contract_context","source_key":"contract_id"}
	]`)

	err := Schema(schema, map[string]any{"q": "hello"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `required parameter "contract_id" is missing`)

	require.NoError(t, Schema(schema, map[string]any{"q": "hello", "contract_id": "C-1"}))

	err = Schema(schema, map[string]any{"q": "hello", "contract_id": 42})
	require.Error(t, err)
	require.Contains(t, err.Error(), `parameter "contract_id" expected string`)
}

func TestSchemaAcceptsExtraUnknownFields(t *testing.T) {
	schema := json.RawMessage(`[
		{"key":"contract_id","type":"string","required":true,"source":"anything","source_key":"ignored","ui_note":"legacy"}
	]`)

	require.NoError(t, Schema(schema, map[string]any{"contract_id": "C-1"}))
}
