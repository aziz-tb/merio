package setting

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

// Getter is the read-only setting surface used by domain packages.
type Getter interface {
	Get(key string) (types.JSONText, error)
}

// IntJSON parses a JSON number or JSON string into an int.
func IntJSON(raw []byte, fallback int) int {
	var i int
	if err := json.Unmarshal(raw, &i); err == nil {
		return i
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return parsed
		}
	}
	return fallback
}

// DurationMinutes reads a setting key as minutes and returns it as a duration.
func DurationMinutes(settings Getter, key string, fallback time.Duration) time.Duration {
	if settings == nil {
		return fallback
	}
	raw, err := settings.Get(key)
	if err != nil || len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return fallback
	}
	minutes, ok := intJSON(raw)
	if !ok || minutes < 0 {
		return fallback
	}
	return time.Duration(minutes) * time.Minute
}

func intJSON(raw []byte) (int, bool) {
	var i int
	if err := json.Unmarshal(raw, &i); err == nil {
		return i, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
