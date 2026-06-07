package capabilities

import (
	"context"
	"fmt"

	"github.com/aziz-tb/merio/internal/ai/tools/builtin"
	"github.com/aziz-tb/merio/internal/contract"
	"github.com/aziz-tb/merio/internal/customer"
	customermodels "github.com/aziz-tb/merio/internal/customer/models"
	"github.com/aziz-tb/merio/internal/customerdata"
)

func init() {
	builtin.RegisterInternal(&getCustomerDataCapability{})
}

// getCustomerDataCapability returns the full customer + contract record
// linked to a conversation — the same payload the agent desk shows in the
// sidebar and that GET /conversations/{uuid}/customer returns. The
// conversation's stored customer_id / contract_id are the verified
// identity, so the tool resolves everything from the conversation rather
// than trusting free-form ids from the model.
type getCustomerDataCapability struct{}

func (g *getCustomerDataCapability) ID() string   { return "get_customer_data" }
func (g *getCustomerDataCapability) Name() string { return "Get the conversation's customer" }

func (g *getCustomerDataCapability) Description() string {
	return "Fetch the full customer and contract record linked to a conversation, including ERP details. " +
		"Returns a customer object (name, contact details, address, segments) plus a `contracts` array; " +
		"each contract carries an `agent_preview` list of human-readable label/value pairs (tariff, meter number, delivery state, consumption, …) alongside the raw ERP fields. " +
		"Available fields vary by what the ERP returns.\n\n" +
		"Use this to address the customer by name, understand their tariff/contract situation, or look up the details needed to draft an accurate reply. " +
		"By default resolves the customer for the conversation currently in scope; pass `conversation_uuid` to look up a different conversation explicitly."
}

func (g *getCustomerDataCapability) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conversation_uuid": map[string]any{
				"type":        "string",
				"description": "Conversation UUID to resolve the customer from. Defaults to the conversation currently in scope.",
			},
		},
	}
}

func (g *getCustomerDataCapability) OutputSchema() map[string]any {
	// The shape mirrors GET /conversations/{uuid}/customer: a flat customer
	// object (Zone 1 + ERP-authored Zone 2 fields) with a nested
	// `contracts` array. Fields are ERP-dependent, so we describe the
	// envelope rather than enumerate every key.
	return map[string]any{
		"type":        "object",
		"description": "Customer record with a nested `contracts` array. Each contract includes an `agent_preview` list of {label, value, tooltip} pairs plus raw ERP fields. Null when no customer is linked to the conversation.",
	}
}

func (g *getCustomerDataCapability) Category() string { return "read" }

func (g *getCustomerDataCapability) Execute(ctx context.Context, params map[string]any, rt *builtin.ToolRuntimeContext) (any, error) {
	if rt == nil || rt.Conversation == nil || rt.Customer == nil || rt.Contract == nil {
		return nil, fmt.Errorf("get_customer_data: runtime conversation/customer/contract managers are nil")
	}

	// Resolve strictly by the provided uuid when given (id=0), otherwise
	// fall back to the conversation currently in scope. get-conversation
	// matches id OR uuid, so leaving both set could resolve the wrong row.
	convUUID, _ := paramString(params, "conversation_uuid")
	lookupID := rt.ConversationID
	if convUUID != "" {
		lookupID = 0
	}
	if lookupID <= 0 && convUUID == "" {
		return nil, fmt.Errorf("get_customer_data: conversation_uuid is required (no conversation in scope and none provided)")
	}

	conv, err := rt.Conversation.GetConversation(lookupID, convUUID, "")
	if err != nil {
		return nil, fmt.Errorf("get_customer_data: %w", err)
	}
	if !conv.CustomerID.Valid || conv.CustomerID.String == "" {
		// No customer linked — mirror the endpoint's empty response.
		return nil, nil
	}
	primaryContractID := ""
	if conv.ContractID.Valid {
		primaryContractID = conv.ContractID.String
	}

	raw, err := customerdata.LoadCustomerWithContracts(ctx, rt.Logf,
		func(c context.Context, id string) (*customermodels.Customer, error) {
			return rt.Customer.GetFreshByCustomerID(c, id, customer.FreshOptions{
				ConversationID: conv.ID,
				TraceID:        rt.TraceID,
			})
		},
		func(c context.Context, ids []string) error {
			return rt.Contract.EnsureHydrated(c, ids, contract.FreshOptions{
				ConversationID: conv.ID,
				TraceID:        rt.TraceID,
			})
		},
		rt.Contract.ListByCustomerID,
		conv.CustomerID.String, primaryContractID)
	if err != nil {
		return nil, fmt.Errorf("get_customer_data: %w", err)
	}

	// json.RawMessage round-trips through the dispatcher's json.Marshal
	// verbatim, so the agent sees byte-identical JSON to the HTTP endpoint.
	return raw, nil
}
