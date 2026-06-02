package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// SystemInboxUUID is the stable identifier for the seeded "System (Internal)"
// inbox used as the FK target for internal ai_usage rows.
const SystemInboxUUID = "00000000-0000-0000-0000-000000000001"

// V0_1_0 is the clean Merio baseline. schema.sql is the source of truth for
// fresh installs, so this migration intentionally performs no DDL.
func V0_1_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	return nil
}
