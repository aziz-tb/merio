package temporal

import (
	sdklog "go.temporal.io/sdk/log"

	"github.com/zerodha/logf"
)

// sdkLogger bridges the project's logf.Logger to Temporal's
// sdklog.Logger interface. Temporal SDK logs flow through the same
// pipeline as everything else (same format, same sink).
type sdkLogger struct {
	lo *logf.Logger
}

// NewSDKLogger returns a Temporal-SDK-compatible Logger backed by
// logf. Used at client and worker construction time via
// client.Options.Logger / worker.Options.Logger.
func NewSDKLogger(lo *logf.Logger) sdklog.Logger {
	return &sdkLogger{lo: lo}
}

func (s *sdkLogger) Debug(msg string, keyvals ...interface{}) {
	s.lo.Debug(msg, keyvals...)
}

func (s *sdkLogger) Info(msg string, keyvals ...interface{}) {
	s.lo.Info(msg, keyvals...)
}

func (s *sdkLogger) Warn(msg string, keyvals ...interface{}) {
	s.lo.Warn(msg, keyvals...)
}

func (s *sdkLogger) Error(msg string, keyvals ...interface{}) {
	s.lo.Error(msg, keyvals...)
}
