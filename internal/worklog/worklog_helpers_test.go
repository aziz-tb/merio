package worklog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

// noopLogger returns a logger that swallows everything below Error so tests
// don't drown stdout in expected-warning noise (e.g. invalid action_type).
func noopLogger() *logf.Logger {
	lo := logf.New(logf.Opts{Level: logf.ErrorLevel})
	return &lo
}

// noopI18n loads the canonical en.json from the repo so manager error paths
// can format messages. Falls back to an empty catalog if the file isn't
// reachable (still satisfies the *i18n.I18n type).
func noopI18n(t *testing.T) *i18n.I18n {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	enPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "i18n", "en.json")
	b, err := os.ReadFile(enPath)
	if err != nil {
		// Empty catalog still works — i18n.T returns the key on miss.
		b = []byte(`{}`)
	}
	in, err := i18n.New(b)
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	return in
}
