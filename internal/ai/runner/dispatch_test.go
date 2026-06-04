package runner

import (
	"errors"
	"testing"

	convmodels "github.com/aziz-tb/merio/internal/conversation/models"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

type recordingInvalidator struct {
	calls []string
	err   error
}

func (r *recordingInvalidator) InvalidateCache(id string) error {
	r.calls = append(r.calls, id)
	return r.err
}

func TestInvalidateCachesForConversationInvalidatesLinkedCustomerAndContract(t *testing.T) {
	customer := &recordingInvalidator{}
	contract := &recordingInvalidator{}
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	r := &Runner{opts: Opts{
		Customer: customer,
		Contract: contract,
		Lo:       &lo,
	}}

	r.invalidateCachesForConversation(convmodels.Conversation{
		ID:         7,
		CustomerID: null.StringFrom("K-11"),
		ContractID: null.StringFrom("C-22"),
	}, "trace-1", "write_tool")

	require.Equal(t, []string{"K-11"}, customer.calls)
	require.Equal(t, []string{"C-22"}, contract.calls)
}

func TestEncodeToolResultPreservesFailureCodeAndRetryable(t *testing.T) {
	out := encodeToolResult(ToolCallLog{
		Status: "failure",
		Error:  "inactive",
		Code:   "INACTIVE_CONTRACT",
	})

	require.JSONEq(t, `{
		"status": "failure",
		"code": "INACTIVE_CONTRACT",
		"error": "inactive",
		"retryable": false
	}`, out)

	out = encodeToolResult(ToolCallLog{
		Status:    "error",
		Error:     "upstream unavailable",
		Retryable: true,
	})

	require.JSONEq(t, `{
		"status": "error",
		"code": null,
		"error": "upstream unavailable",
		"retryable": true
	}`, out)
}

func TestInvalidateCachesForConversationSkipsMissingIDsAndContinuesAfterError(t *testing.T) {
	customer := &recordingInvalidator{}
	contract := &recordingInvalidator{err: errors.New("boom")}
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	r := &Runner{opts: Opts{
		Customer: customer,
		Contract: contract,
		Lo:       &lo,
	}}

	r.invalidateCachesForConversation(convmodels.Conversation{
		ID:         7,
		CustomerID: null.StringFrom("K-11"),
		ContractID: null.StringFrom("C-22"),
	}, "trace-1", "write_tool")
	r.invalidateCachesForConversation(convmodels.Conversation{ID: 8}, "trace-2", "read_tool")

	require.Equal(t, []string{"K-11"}, customer.calls)
	require.Equal(t, []string{"C-22"}, contract.calls)
}
