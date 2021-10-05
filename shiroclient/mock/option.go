package mock

import (
	"io"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/mockint"
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
