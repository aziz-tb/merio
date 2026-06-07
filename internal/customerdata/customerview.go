package customerdata

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	contractmodels "github.com/aziz-tb/merio/internal/contract/models"
	customermodels "github.com/aziz-tb/merio/internal/customer/models"
)

// Warner is the small slice of a logger used to report non-fatal
// hydration failures. *logf.Logger and the domain managers satisfy it; a
// nil Warner silences the warning.
type Warner interface {
	Warn(string, ...any)
}

// CustomerFetcher loads a fresh customer by ERP customer id.
type CustomerFetcher func(ctx context.Context, customerID string) (*customermodels.Customer, error)

// ContractHydrator ensures the given ERP contract ids are cached and fresh.
type ContractHydrator func(ctx context.Context, contractIDs []string) error

// ContractLister returns every cached contract for a customer id.
type ContractLister func(customerID string) ([]contractmodels.Contract, error)

// LoadCustomerWithContracts fetches a fresh customer, hydrates its
// contracts, and serializes the combined Zone 1 + Zone 2 shape via
// CustomerToAPIResponse. It is the single source of truth behind the
// GET /conversations/{uuid}/customer endpoint and the get_customer_data
// agent tool, so both return byte-identical data. The fetch/hydrate/list
// adapters let callers bind their own FreshOptions without this package
// importing the customer/contract managers (which would cycle).
func LoadCustomerWithContracts(ctx context.Context, lo Warner, fetch CustomerFetcher, hydrate ContractHydrator, list ContractLister, customerID, primaryContractID string) (json.RawMessage, error) {
	cust, err := fetch(ctx, customerID)
	if err != nil {
		return nil, err
	}
	contracts, err := LoadContractsForCustomer(ctx, lo, hydrate, list, cust, primaryContractID)
	if err != nil {
		return nil, err
	}
	return CustomerToAPIResponse(*cust, contracts), nil
}

// LoadContractsForCustomer hydrates and returns the contracts belonging to
// a customer. When primaryContractID is set it hydrates only that contract
// (the conversation's verified contract); otherwise it hydrates every id
// listed on the customer. Hydration failures are logged and skipped so a
// partial list is still returned.
func LoadContractsForCustomer(ctx context.Context, lo Warner, hydrate ContractHydrator, list ContractLister, cust *customermodels.Customer, primaryContractID string) ([]contractmodels.Contract, error) {
	hydrateIDs := ContractIDsFromRaw(cust.ContractIDs)
	primaryContractID = strings.TrimSpace(primaryContractID)
	if primaryContractID != "" {
		hydrateIDs = []string{primaryContractID}
	}
	if len(hydrateIDs) > 0 {
		if err := hydrate(ctx, hydrateIDs); err != nil && lo != nil {
			lo.Warn("contract hydration cascade failed; returning partial data", "customer_id", cust.CustomerID, "error", err)
		}
	}
	return list(cust.CustomerID)
}

// ContractIDsFromRaw decodes a customer's contract_ids JSON array into a
// deduplicated slice of non-empty string ids, coercing numeric ids to
// strings.
func ContractIDsFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := ciValueString(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func ciValueString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}
