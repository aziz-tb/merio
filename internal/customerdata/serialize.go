package customerdata

import (
	"bytes"
	"encoding/json"

	contractmodels "github.com/abhinavxd/libredesk/internal/contract/models"
	customermodels "github.com/abhinavxd/libredesk/internal/customer/models"
	"github.com/volatiletech/null/v9"
)

// CustomerToAPIResponse serializes the flat Zone 1 + Zone 2 customer shape used
// by public APIs and detection variables. Zone 2 fields are appended in their
// original ERP-authored order.
func CustomerToAPIResponse(c customermodels.Customer, contracts []contractmodels.Contract) json.RawMessage {
	var out bytes.Buffer
	out.WriteByte('{')
	writeJSONField(&out, "customer_id", c.CustomerID, true)
	writeJSONField(&out, "business", c.Business, false)
	writeJSONField(&out, "firstname", nullStringValue(c.Firstname), false)
	writeJSONField(&out, "lastname", nullStringValue(c.Lastname), false)
	writeJSONField(&out, "company", nullStringValue(c.Company), false)
	writeJSONField(&out, "email", nullStringValue(c.Email), false)
	writeJSONField(&out, "phone", nullStringValue(c.Phone), false)
	writeJSONField(&out, "zip", nullStringValue(c.Zip), false)
	writeJSONField(&out, "delivery_address", nullStringValue(c.DeliveryAddress), false)
	writeJSONField(&out, "billing_address", nullStringValue(c.BillingAddress), false)
	writeJSONFieldRaw(&out, "contract_ids", c.ContractIDs, false)
	writeJSONField(&out, "created_at", c.CreatedAt, false)
	writeJSONField(&out, "updated_at", c.UpdatedAt, false)
	writeJSONField(&out, "last_synced_at", c.LastSyncedAt, false)
	appendZone2Fields(&out, c.ExtraData)
	if contracts != nil {
		out.WriteString(`,"contracts":[`)
		for i, ct := range contracts {
			if i > 0 {
				out.WriteByte(',')
			}
			out.Write(ContractToAPIResponse(ct))
		}
		out.WriteByte(']')
	}
	out.WriteByte('}')
	return out.Bytes()
}

// ContractToAPIResponse serializes the flat Zone 1 + Zone 2 contract shape used
// by public APIs and detection variables.
func ContractToAPIResponse(c contractmodels.Contract) json.RawMessage {
	var out bytes.Buffer
	out.WriteByte('{')
	writeJSONField(&out, "contract_id", c.ContractID, true)
	writeJSONField(&out, "customer_id", c.CustomerID, false)
	writeJSONField(&out, "business", c.Business, false)
	writeJSONField(&out, "firstname", nullStringValue(c.Firstname), false)
	writeJSONField(&out, "lastname", nullStringValue(c.Lastname), false)
	writeJSONField(&out, "company", nullStringValue(c.Company), false)
	writeJSONField(&out, "email", nullStringValue(c.Email), false)
	writeJSONField(&out, "phone", nullStringValue(c.Phone), false)
	writeJSONField(&out, "zip", nullStringValue(c.Zip), false)
	writeJSONField(&out, "delivery_address", nullStringValue(c.DeliveryAddress), false)
	writeJSONField(&out, "billing_address", nullStringValue(c.BillingAddress), false)
	writeJSONFieldRaw(&out, "agent_preview", c.AgentPreview, false)
	writeJSONFieldRaw(&out, "erp_urls", c.ErpURLs, false)
	writeJSONField(&out, "created_at", c.CreatedAt, false)
	writeJSONField(&out, "updated_at", c.UpdatedAt, false)
	writeJSONField(&out, "last_synced_at", c.LastSyncedAt, false)
	appendZone2Fields(&out, c.ExtraData)
	out.WriteByte('}')
	return out.Bytes()
}

func CustomerToMap(c customermodels.Customer, contracts []contractmodels.Contract) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(CustomerToAPIResponse(c, contracts), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func ContractToMap(c contractmodels.Contract) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(ContractToAPIResponse(c), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeJSONField(out *bytes.Buffer, key string, value any, first bool) {
	if !first {
		out.WriteByte(',')
	}
	keyJSON, _ := json.Marshal(key)
	valueJSON, _ := json.Marshal(value)
	out.Write(keyJSON)
	out.WriteByte(':')
	out.Write(valueJSON)
}

func writeJSONFieldRaw(out *bytes.Buffer, key string, raw json.RawMessage, first bool) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`null`)
	}
	if !first {
		out.WriteByte(',')
	}
	keyJSON, _ := json.Marshal(key)
	out.Write(keyJSON)
	out.WriteByte(':')
	out.Write(raw)
}

func appendZone2Fields(out *bytes.Buffer, extraData json.RawMessage) {
	if len(extraData) == 0 {
		return
	}
	trimmed := bytes.TrimSpace(extraData)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(extraData))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return
		}
		key, ok := keyTok.(string)
		if !ok {
			return
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return
		}
		out.WriteByte(',')
		keyJSON, _ := json.Marshal(key)
		out.Write(keyJSON)
		out.WriteByte(':')
		out.Write(value)
	}
}

func nullStringValue(n null.String) any {
	if !n.Valid {
		return nil
	}
	return n.String
}
