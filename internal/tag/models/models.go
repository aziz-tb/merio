package models

import (
	"time"

	"github.com/abhinavxd/libredesk/internal/detection"
	"github.com/volatiletech/null/v9"
)

const (
	TypeSkill   = "skill"
	TypeTopic   = "topic"
	TypeContact = "contact"
	TypeOther   = "other"

	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityDisabled = "disabled"

	AppliedByManual = "manual"
	AppliedByAI     = "ai"
	AppliedByRule   = "rule"
)

// Tag represents a tag with its detection configuration.
type Tag struct {
	ID            int                 `db:"id" json:"id"`
	CreatedAt     time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time           `db:"updated_at" json:"updated_at"`
	Name          string              `db:"name" json:"name"`
	ParentID      null.Int            `db:"parent_id" json:"parent_id"`
	Type          string              `db:"type" json:"type"`
	Color         string              `db:"color" json:"color"`
	Visibility    string              `db:"visibility" json:"visibility"`
	Description   string              `db:"description" json:"description"`
	Detection     detection.Detection `db:"detection" json:"detection"`
	SLAPolicyID   null.Int            `db:"sla_policy_id" json:"sla_policy_id"`
	PriorityID    null.Int            `db:"priority_id" json:"priority_id"`
	UpdatedBy     null.Int            `db:"updated_by" json:"updated_by"`
	UpdatedByName string              `db:"updated_by_name" json:"updated_by_name,omitempty"`
}

func (t *Tag) SetDefaults() {
	if t.Type == "" {
		t.Type = TypeSkill
	}
	if t.Color == "" {
		t.Color = "grey"
	}
	if t.Visibility == "" {
		t.Visibility = VisibilityPublic
	}
	if t.Detection.Mode == "" {
		t.Detection.Mode = detection.ModeManualOnly
	}
}
