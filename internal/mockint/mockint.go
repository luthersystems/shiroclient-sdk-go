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
	// SharedPlugin hosts the mock in a plugin process shared with other
	// mocks that use the same plugin path, log level and log writer.
	SharedPlugin bool
	// SharedIdleTimeout is how long a shared plugin process stays alive
	// after its last mock closes. Zero stops it immediately.
	SharedIdleTimeout time.Duration
	// Creator is the fake transaction creator MSP ID set on the mock
	// ledger when it is created. Empty leaves the creator unset.
	Creator string
}

// DefaultCreator is the default Creator: the fake transaction creator MSP
// ID a new mock sets, so a phylum's MSP checks (cc:creator) run in memory.
const DefaultCreator = "Org1MSP"

// DefaultSharedIdleTimeout is the default SharedIdleTimeout.
const DefaultSharedIdleTimeout = 10 * time.Second
