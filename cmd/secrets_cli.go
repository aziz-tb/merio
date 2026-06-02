package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aziz-tb/merio/internal/secret"
	"github.com/jmoiron/sqlx"
)

func runSecretsCLI(ctx context.Context, args []string) {
	if len(args) == 0 {
		printSecretsCLIUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "rotate-kek":
		runSecretsRotateKEK(ctx, args[1:])
	case "verify":
		runSecretsVerify(ctx, args[1:])
	default:
		printSecretsCLIUsage()
		os.Exit(1)
	}
}

func runSecretsRotateKEK(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("merio secrets rotate-kek", flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file")
	oldRaw := fs.String("old-kek", "", "old base64-encoded 32-byte KEK")
	newRaw := fs.String("new-kek", "", "new base64-encoded 32-byte KEK")
	_ = fs.Parse(args)

	loadSecretsCLIConfig(*configPath)
	db := initDB()
	ensureSecretsCLISchema(db)

	oldValue := readSecretsCLIValue("old KEK", *oldRaw)
	newValue := readSecretsCLIValue("new KEK", *newRaw)
	oldKEK, err := secret.DecodeKEK(strings.TrimSpace(oldValue))
	if err != nil {
		log.Fatalf("invalid old KEK: %v", err)
	}
	newKEK, err := secret.DecodeKEK(strings.TrimSpace(newValue))
	if err != nil {
		log.Fatalf("invalid new KEK: %v", err)
	}

	n, err := secret.RotateKEK(ctx, db, oldKEK, newKEK)
	if err != nil {
		log.Fatalf("rotate KEK: %v", err)
	}
	fmt.Printf("Rotated %d secrets. Update %s env var to the new value and restart.\n", n, secret.KEKEnvVar)
}

func runSecretsVerify(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("merio secrets verify", flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file")
	_ = fs.Parse(args)

	loadSecretsCLIConfig(*configPath)
	db := initDB()
	ensureSecretsCLISchema(db)

	kek, err := secret.LoadKEK()
	if err != nil {
		log.Fatalf("load KEK: %v", err)
	}
	failures, err := secret.VerifyKEK(ctx, db, kek)
	if err != nil {
		log.Fatalf("verify secrets: %v", err)
	}
	if len(failures) > 0 {
		fmt.Printf("Failed to decrypt %d secrets: %s\n", len(failures), strings.Join(failures, ", "))
		os.Exit(1)
	}
	fmt.Println("All secrets decrypted successfully.")
}

func loadSecretsCLIConfig(configPath string) {
	ko.Set("config", []string{configPath})
	initConfig(ko)
	validateConfig(ko)
}

func ensureSecretsCLISchema(db *sqlx.DB) {
	installed, err := checkSchema(db)
	if err != nil {
		log.Fatalf("error checking db schema: %v", err)
	}
	if !installed {
		log.Fatal("database tables are missing. Run merio --install first.")
	}
}

func readSecretsCLIValue(label, current string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	fmt.Fprintf(os.Stderr, "%s: ", label)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		log.Fatalf("failed reading %s", label)
	}
	return scanner.Text()
}

func printSecretsCLIUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  merio secrets rotate-kek --old-kek <base64> --new-kek <base64>
  merio secrets verify

Options:
  --config <path>    Path to config file (default: config.toml)
`)
}
