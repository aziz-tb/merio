// Package models contains the data models for the customer package.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Customer is a customers row: a cache of an ERP-resolved customer. Stable fields are typed
// columns; everything else lives in ExtraData. Business selects the firstname/lastname-vs-company
// branching enforced by the customers business-consistency CHECK.
type Customer struct {
	ID              int64           `db:"id" json:"-"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
	LastSyncedAt    time.Time       `db:"last_synced_at" json:"last_synced_at"`
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
	ContractIDs     json.RawMessage `db:"contract_ids" json:"contract_ids"`
	ExtraData       json.RawMessage `db:"extra_data" json:"-"`
}
