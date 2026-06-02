package erp

import (
	"encoding/json"
	"fmt"
	"strings"

	custmodels "github.com/abhinavxd/libredesk/internal/customer/models"
	"github.com/abhinavxd/libredesk/internal/customerdata"
	"github.com/volatiletech/null/v9"
)

var zone1CustomerKeys = map[string]bool{
	"id": true, "customerId": true, "customer_id": true,
	"business": true,
	"lastname": true, "lastName": true, "surname": true,
	"company": true, "companyName": true,
	"contractIds": true, "contract_ids": true,
	"firstname": true, "firstName": true,
	"email": true, "mail": true,
	"phone": true, "phoneNumber": true,
	"zip": true, "postalCode": true,
	"deliveryAddress": true, "delivery_address": true, "address": true,
	"billingAddress": true, "billing_address": true,
}

// FromExecutorData normalizes customer-shaped ERP executor output. Zone 1
// fields are extracted into typed columns and every non-Zone-1 key is kept as
// ordered, original-casing Zone 2 data in ExtraData.
func FromExecutorData(raw json.RawMessage, fallbackCustomerID string) (custmodels.Customer, bool, error) {
	raw, data, err := customerdata.UnwrapObject(raw, "customer")
	if err != nil {
		return custmodels.Customer{}, false, fmt.Errorf("parse customer executor output: %w", err)
	}
	if len(data) == 0 {
		return custmodels.Customer{}, false, nil
	}

	customerID := customerdata.FirstString(data, "customer_id", "customerId", "id")
	if customerID == "" {
		customerID = fallbackCustomerID
	}
	if customerID == "" {
		return custmodels.Customer{}, false, fmt.Errorf("customer executor output missing customer id")
	}

	business, found, err := customerdata.FirstBool(data, "business")
	if err != nil {
		return custmodels.Customer{}, false, fmt.Errorf("business is required: %w", err)
	}
	if !found {
		return custmodels.Customer{}, false, fmt.Errorf("business is required")
	}

	firstname := customerdata.FirstString(data, "firstname", "firstName")
	lastname := customerdata.FirstString(data, "lastname", "lastName", "surname")
	company := customerdata.FirstString(data, "company", "companyName")
	if business {
		if strings.TrimSpace(company) == "" {
			return custmodels.Customer{}, false, fmt.Errorf("business customer requires company")
		}
	} else if strings.TrimSpace(lastname) == "" {
		return custmodels.Customer{}, false, fmt.Errorf("private customer requires lastname")
	}

	contractIDs, found, err := customerdata.FirstStringSlice(data, "contractIds", "contract_ids")
	if err != nil {
		return custmodels.Customer{}, false, fmt.Errorf("contractIds must be array: %w", err)
	}
	if !found || len(contractIDs) == 0 {
		return custmodels.Customer{}, false, fmt.Errorf("customer requires at least one contract")
	}
	contractIDsBytes, _ := json.Marshal(contractIDs)

	extraData := customerdata.BuildRemainder(raw, zone1CustomerKeys)

	return custmodels.Customer{
		CustomerID:      customerID,
		Business:        business,
		Firstname:       nullString(firstname),
		Lastname:        nullString(lastname),
		Company:         nullString(company),
		Email:           nullString(customerdata.FirstString(data, "email", "mail")),
		Phone:           nullString(customerdata.FirstString(data, "phone", "phoneNumber")),
		Zip:             nullString(customerdata.FirstString(data, "zip", "postalCode")),
		DeliveryAddress: nullString(customerdata.FirstString(data, "deliveryAddress", "delivery_address", "address")),
		BillingAddress:  nullString(customerdata.FirstString(data, "billingAddress", "billing_address")),
		ContractIDs:     contractIDsBytes,
		ExtraData:       extraData,
	}, true, nil
}

func nullString(s string) null.String {
	if strings.TrimSpace(s) == "" {
		return null.String{}
	}
	return null.StringFrom(strings.TrimSpace(s))
}
