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

// WithSharedPlugin hosts the mock in a substratehcp plugin process shared
// with other mocks, instead of starting a process per mock.
//
// Mocks share a process when they use the same plugin path, log level and
// log writer (a log writer whose type is not comparable disables sharing for
// that mock). Each mock keeps its own ledger and server inside the process,
// addressed by its own handle, so mocks do not see each other's state. Close
// releases only that mock. The process stops once its last mock has closed
// and the idle timeout (WithSharedPluginIdleTimeout) has passed, or when
// shiroclient.ShutdownSharedMockPlugins is called.
//
// Trade-offs: a plugin crash fails every mock the process hosts, and the
// plugin's output goes to one shared writer. Memory also behaves
// differently: a short-lived process returns its garbage to the system when
// it exits, but a shared process keeps a Go heap sized for the mocks it has
// hosted, so under heavy churn its peak RSS can exceed that of per-process
// mode even though it saves CPU and start-up time. Without this option each mock
// gets its own process, as before.
func WithSharedPlugin() Option {
	return func(config *mockint.Config) {
		config.SharedPlugin = true
	}
}

// WithSharedPluginIdleTimeout sets how long a shared plugin process stays
// alive after its last mock closes, so that a suite that creates and closes
// mocks one after another reuses one process (the default is 10s). Zero
// stops the process as soon as its last mock closes. It has no effect
// without WithSharedPlugin; the most recent mock to join a process sets it.
func WithSharedPluginIdleTimeout(d time.Duration) Option {
	return func(config *mockint.Config) {
		if d < 0 {
			d = 0
		}
		config.SharedIdleTimeout = d
	}
}
