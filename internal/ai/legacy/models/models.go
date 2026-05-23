package models

import "time"

type Provider struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Name      string    `db:"name"`
	Provider  string    `db:"provider"`
	Config    string    `db:"config"`
	IsDefault bool      `db:"is_default"`
}

type Prompt struct {
	ID        int       `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Title     string    `db:"title" json:"title"`
	Key       string    `db:"key" json:"key"`
	Content   string    `db:"content" json:"content,omitempty"`
}
