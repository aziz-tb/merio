// Package anthropic implements LLMProvider against Anthropic's official
// SDK. The package owns retry, circuit breaking, bounded concurrency,
// prompt caching, and structured observability so callers (agent runner,
// pipeline stages) only see Complete().
package anthropic

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abhinavxd/libredesk/internal/crypto"
	"github.com/jmoiron/sqlx"
)

// LookupAPIKey reads the encrypted Anthropic API key from the
// ai_providers table and decrypts it with the app encryption key. Mirrors
// the existing OpenAI rewrite manager's pattern (internal/ai/ai.go) so
// there's one storage convention for all provider credentials.
//
// Returns ("", nil) when no row exists yet — callers can treat this as
// "configured, but admin hasn't set the key" and skip provider init or
// surface a clear "set the key in admin → AI providers" error.
func LookupAPIKey(db *sqlx.DB, encryptionKey string) (string, error) {
	var configRaw string
	err := db.QueryRow(`SELECT config FROM ai_providers WHERE provider = 'anthropic' LIMIT 1`).Scan(&configRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("anthropic: querying ai_providers: %w", err)
	}
	var cfg struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(configRaw), &cfg); err != nil {
		return "", fmt.Errorf("anthropic: parsing ai_providers.config: %w", err)
	}
	if cfg.APIKey == "" {
		return "", nil
	}
	plain, err := crypto.Decrypt(cfg.APIKey, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("anthropic: decrypting api_key: %w", err)
	}
	return plain, nil
}
