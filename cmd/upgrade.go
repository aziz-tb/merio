// Copyright Kailash Nadh (https://github.com/knadh/listmonk)
// SPDX-License-Identifier: AGPL-3.0
// Adapted from listmonk for Merio.

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/aziz-tb/merio/internal/dbutil"
	"github.com/aziz-tb/merio/internal/migrations"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"golang.org/x/mod/semver"
)

// migFunc represents a migration function for a particular version.
// fn (generally) executes database migrations and additionally
// takes the filesystem and config objects in case there are additional bits
// of logic to be performed before executing upgrades. fn is idempotent.
type migFunc struct {
	version string
	fn      func(*sqlx.DB, stuffbin.FileSystem, *koanf.Koanf) error
}

// migList is the ordered list of available database migrations.
// Merio starts from a clean schema.sql baseline, so only the baseline marker
// remains here. Future schema changes can append forward-only migrations.
var migList = []migFunc{
	{"v0.1.0", migrations.V0_1_0},
	{"v0.2.0", migrations.V0_2_0},
}

// upgrade upgrades the database to the current version by running SQL migration files
// for all version from the last known version to the current one.
func upgrade(db *sqlx.DB, fs stuffbin.FileSystem, prompt bool) {
	if prompt {
		var ok string
		fmt.Printf("** IMPORTANT: Take a backup of the database before upgrading.\n")
		fmt.Print("continue (y/n)?  ")
		if _, err := fmt.Scanf("%s", &ok); err != nil {
			log.Fatalf("error reading value from terminal: %v", err)
		}
		if !strings.EqualFold(ok, "y") {
			fmt.Println("upgrade cancelled")
			return
		}
	}

	_, toRun, err := getPendingMigrations(db)
	if err != nil {
		log.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		log.Printf("no pending upgrades. Database is up to date.")
		return
	}

	// Execute migrations in succession.
	for _, m := range toRun {
		log.Printf("running migration %s", m.version)
		if err := m.fn(db, fs, ko); err != nil {
			log.Fatalf("error running migration %s: %v", m.version, err)
		}

		// Record the migration version in the settings table.
		if err := recordMigrationVersion(m.version, db); err != nil {
			if dbutil.IsTableNotExistError(err) {
				continue
			}
			log.Fatalf("error recording migration version %s: %v", m.version, err)
		}
	}

	log.Printf("upgrade complete")
}

// getPendingMigrations gets the pending migrations by comparing the last
// recorded migration in the DB against all migrations listed in `migrations`.
func getPendingMigrations(db *sqlx.DB) (string, []migFunc, error) {
	lastVer, err := getLastMigrationVersion(db)
	if err != nil {
		return "", nil, err
	}

	// Iterate through the migration versions and get everything above the last
	// upgraded semver.
	var toRun []migFunc
	for i, m := range migList {
		if semver.Compare(m.version, lastVer) > 0 {
			toRun = migList[i:]
			break
		}
	}

	return lastVer, toRun, nil
}

func latestMigrationVersion() string {
	if len(migList) == 0 {
		return "v0.0.0"
	}
	return migList[len(migList)-1].version
}

// getLastMigrationVersion returns the last migration semver recorded in the DB.
// If there isn't any, `v0.0.0` is returned.
func getLastMigrationVersion(db *sqlx.DB) (string, error) {
	var v string
	if err := db.Get(&v, `
		SELECT COALESCE(
			(SELECT value->>-1 FROM settings WHERE key='migrations'),
		'v0.0.0')`); err != nil {
		if dbutil.IsTableNotExistError(err) {
			return "v0.0.0", nil
		}
		return v, err
	}
	return v, nil
}

// recordMigrationVersion inserts the given version (of DB migration) into the
// `migrations` array in the settings table.
func recordMigrationVersion(ver string, db *sqlx.DB) error {
	_, err := db.Exec(`INSERT INTO settings (key, value)
	VALUES('migrations', jsonb_build_array($1::text))
	ON CONFLICT (key) DO UPDATE SET value = settings.value || EXCLUDED.value`, ver)
	return err
}

// recordCurrentMigrationVersion marks a freshly installed schema.sql database
// as already being at the current baseline.
func recordCurrentMigrationVersion(db *sqlx.DB) error {
	latest := latestMigrationVersion()
	if latest == "v0.0.0" {
		return nil
	}
	_, err := db.Exec(`INSERT INTO settings (key, value)
	VALUES('migrations', jsonb_build_array($1::text))
	ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, latest)
	return err
}

// checkPendingUpgrade checks if the current database schema matches the expected binary version.
func checkPendingUpgrade(db *sqlx.DB) {
	lastVer, toRun, err := getPendingMigrations(db)
	if err != nil {
		log.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		return
	}

	var vers []string
	for _, m := range toRun {
		vers = append(vers, m.version)
	}

	log.Fatalf(`there are %d pending database upgrade(s): %v. The last upgrade was %s. Backup the database and run merio --upgrade`,
		len(toRun), vers, lastVer)
}
