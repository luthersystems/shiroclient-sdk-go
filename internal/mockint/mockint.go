package mockint

import (
	"io"
	"time"
)

const (
	// DefaultPluginEnv is the environment variable used to locate the HCP
	// plugin by default
	DefaultPluginEnv = "SUBSTRATEHCP_FILE"
	// PhylumName is the name of the mock phylum
	PhylumName = "test"
	// PhylumVersion is the version of the mock phylum
	PhylumVersion = "test"
)

// LogLevel is a type to control the plugin log level
type LogLevel int

// Config is the internal configuration for the mock client
type Config struct {
	PluginPath     string
	LogWriter      io.Writer
	LogLevel       LogLevel
	SnapshotReader io.Reader
	// PreheatTimeout overrides the substrate phylum preheat/init timeout. A
	// non-positive value leaves the substrate default in effect.
	PreheatTimeout time.Duration
}
