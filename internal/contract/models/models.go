// Package models contains the data models for the contract package.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Contract is a contracts row: a cache of an ERP-resolved contract belonging to a customer.
// CustomerID is the ERP FK to customers(customer_id); Email/Phone may differ from the parent
// customer's. AgentPreview holds the sidebar tiles, ErpURLs the quick-link buttons.
type Contract struct {
	ID              int64           `db:"id" json:"-"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	LastSyncedAt    time.Time       `db:"last_synced_at" json:"last_synced_at"`
	ContractID      string          `db:"contract_id" json:"contract_id"`
	CustomerID      string          `db:"customer_id" json:"customer_id"`
	Business        bool            `db:"business" json:"business"`
	Firstname       null.String     `db:"firstname" json:"firstname"`
	Lastname        null.String     `db:"lastname" json:"lastname"`
	Company         null.String     `db:"company" json:"company"`
	Email           null.String     `db:"email" json:"email"`
	Phone           null.String     `db:"phone" json:"phone"`
	Zip             null.String     `db:"zip" json:"zip"`
	DeliveryAddress null.String     `db:"delivery_address" json:"delivery_address"`
	BillingAddress  null.String     `db:"billing_address" json:"billing_address"`
	AgentPreview    json.RawMessage `db:"agent_preview" json:"agent_preview"`
	ErpURLs         json.RawMessage `db:"erp_urls" json:"erp_urls"`
	ExtraData       json.RawMessage `db:"extra_data" json:"-"`
}
