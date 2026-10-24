// Package buildinfo holds release metadata shared by the CLI and API.
package buildinfo

// Version is overridden by release builds with -ldflags.
var Version = "0.1.0-dev"
