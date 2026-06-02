package customeridentification

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	execmodels "github.com/abhinavxd/libredesk/internal/exec/runner/models"
	"github.com/abhinavxd/libredesk/internal/function"
	cimodels "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
)

func (a *Activities) SearchCustomerByFieldsActivity(ctx context.Context, in cimodels.SearchCustomerByFieldsInput) (cimodels.SearchCustomerByFieldsOutput, error) {
	input := map[string]any{}
	for k, v := range in.Attrs {
		if strings.TrimSpace(k) != "" && v != nil {
			input[k] = v
		}
	}
	input["ticketId"] = in.ConversationID

	out := cimodels.SearchCustomerByFieldsOutput{Input: input}
	res, err := a.Function.Run(ctx, function.RunRequest{
		Name:           "search_customer_by_fields",
		Input:          input,
		InvokerUserID:  intPtrToInt(in.TriggeredByUserID),
		ConversationID: int(in.ConversationID),
		TraceID:        in.TraceID,
	})
	if err != nil {
		return out, err
	}
	if res == nil {
		return out, nil
	}
	decoded, err := res.DecodedData()
	if err != nil {
		return out, fmt.Errorf("decode search_customer_by_fields result: %w", err)
	}
	data, multiple := searchResultMap(decoded)
	if data == nil || len(data) == 0 || res.Kind == execmodels.KindNoop {
		return out, nil
	}
	if res.Kind != execmodels.KindSuccess {
		return out, fmt.Errorf("search_customer_by_fields returned %s: %s", res.Kind, res.Message)
	}
	out.Output = data
	if multiple || hasMultipleMatches(data) {
		out.MultipleMatchesContractViolation = true
		if a.Lo != nil {
			a.Lo.Error("search_customer_by_fields returned multiple matches", "trace_id", in.TraceID)
		}
		return out, nil
	}

	if nested, ok := asMap(data["match"]); ok {
		data = nested
		out.Output = nested
	}
	if id := firstString(data, "contractId", "contract_id"); id != "" {
		out.ContractID = &id
	}
	if id := firstString(data, "customerId", "customer_id"); id != "" {
		out.CustomerID = &id
	}
	if raw := rawJSONFrom(data, "agentPreview", "agent_preview"); len(raw) > 0 {
		out.AgentPreview = raw
	}
	if level, ok := intFromAny(data["customerVerification"]); ok {
		out.CustomerVerification = &level
	} else if level, ok := intFromAny(data["customer_verification"]); ok {
		out.CustomerVerification = &level
	}
	if msg := firstString(data, "customerVerificationMessage", "customer_verification_message"); msg != "" {
		out.CustomerVerificationMessage = &msg
	}
	return out, nil
}

func searchResultMap(v any) (map[string]any, bool) {
	if m, ok := asMap(v); ok {
		return m, false
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	if len(items) != 1 {
		return nil, len(items) > 1
	}
	m, ok := asMap(items[0])
	return m, !ok
}

func hasMultipleMatches(data map[string]any) bool {
	for _, key := range []string{"matches", "customers", "contracts", "results"} {
		if arr, ok := data[key].([]any); ok && len(arr) > 1 {
			return true
		}
	}
	return false
}

func intFromAny(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		i, err := strconv.Atoi(x.String())
		return i, err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(x))
		return i, err == nil
	default:
		return 0, false
	}
}
