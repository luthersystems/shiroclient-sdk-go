// Package mock provides utilities for configuring a mock (in-memory)
// shiroclient.
package mock

import (
	"io"
	"time"

	"github.com/luthersystems/shiroclient-sdk-go/internal/mockint"
)

const (
	// Debug sets the plugin log level to debug
	Debug mockint.LogLevel = iota
	// Info sets the plugin log level to info
	Info
	// Warn sets the plugin log level to warning
	Warn
	// Error sets the plugin log level to error
	Error
)

// Option is a mock client configuration function
type Option func(*mockint.Config)

// WithPluginPath sets the path to the HCP plugin file.  By default, the plugin
// is loaded from the location specified by the SUBSTRATEHCP_FILE environment
// variable.
func WithPluginPath(path string) Option {
	return func(config *mockint.Config) {
		config.PluginPath = path
	}
}

// WithLogWriter sets the plugin's log destination to the supplied io.Writer.
// By default, the plugin writes to os.Stdout.
//
// This covers both streams the plugin produces: the subprocess's own stdout
// and stderr, and go-plugin's host-side client logger ("starting plugin",
// handshake progress, and error-level reports such as a plugin exiting
// unexpectedly). Before, it reached only the first, so the second went to
// os.Stdout however this was set -- passing io.Discard did not actually
// silence the plugin.
func WithLogWriter(w io.Writer) Option {
	return func(config *mockint.Config) {
		config.LogWriter = w
	}
}

// WithLogLevel sets the log level of the plugin log writer to the supplied
// level.
func WithLogLevel(level mockint.LogLevel) Option {
	return func(config *mockint.Config) {
		config.LogLevel = level
	}
}

// WithSnapshotReader initializes the state of the mock client by reading a
// snapshot of previous state from the supplied io.Reader that was previously
// created with the Snapshot method.
func WithSnapshotReader(r io.Reader) Option {
	return func(config *mockint.Config) {
		config.SnapshotReader = r
	}
}

// WithPreheatTimeout overrides the substrate phylum preheat/init timeout for
// the mock. A non-positive duration leaves the substrate default (6s) in
// effect. Raising it helps avoid spurious "phylum init timeout" errors when
// many mock clients initialize in parallel under heavy CPU load, e.g. large Go
// test suites running with high parallelism.
//
// This requires a substratehcp plugin built from a substrate version that
// honors the option; older plugins silently ignore it.
func WithPreheatTimeout(d time.Duration) Option {
	return func(config *mockint.Config) {
		config.PreheatTimeout = d
	}
}
