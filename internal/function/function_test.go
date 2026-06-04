package function

import (
	"testing"

	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/stretchr/testify/require"
)

type fakeConversationStore struct {
	row convmodels.Conversation
	err error
}

func (f fakeConversationStore) GetConversation(int, string, string) (convmodels.Conversation, error) {
	return f.row, f.err
}

func TestBuildContextPayloadSetsConversationUUIDOnly(t *testing.T) {
	m := &Manager{}
	m.SetConversationStore(fakeConversationStore{row: convmodels.Conversation{
		ID:   7,
		UUID: "conv-uuid",
	}})
	input := map[string]any{"manual_contract_id": "override"}

	payload, err := m.buildContextPayload(RunRequest{
		Input:          input,
		ConversationID: 7,
	})

	require.NoError(t, err)
	require.Equal(t, "conv-uuid", payload.ConversationUUID)
	require.Equal(t, "override", payload.Input["manual_contract_id"])
	payload.Input["manual_contract_id"] = "changed"
	require.Equal(t, "override", input["manual_contract_id"])
}

func TestBuildContextPayloadDoesNotInjectContextParams(t *testing.T) {
	m := &Manager{}
	m.SetConversationStore(fakeConversationStore{row: convmodels.Conversation{
		ID:   7,
		UUID: "conv-uuid",
	}})

	payload, err := m.buildContextPayload(RunRequest{
		ConversationID: 7,
	})

	require.NoError(t, err)
	require.NotContains(t, payload.Input, "contract_id")
	require.NotContains(t, payload.Input, "customer_id")
}
