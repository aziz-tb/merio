// Package version exposes the application's build version, injected at link
// time via -ldflags -X. It is a leaf package (no imports) so any component can
// reference the version without creating import cycles.
package version

// Version holds the current version of the application.
// This value is intended to be set at build time using the -X linker flag.
var Version = "unknown-version"
