package plugin

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNewSubstrateConnectionReturnsErrors pins that a plugin which cannot be
// started is REPORTED rather than fatal.
//
// Both failure paths in NewSubstrateConnection used to be log.Fatal, which
// writes to the standard logger and then calls os.Exit(1) -- from a library,
// on a function that declares an error return and whose caller
// (internal/mock.NewMock) propagates it. A plugin that failed to start killed
// the caller's process: no deferred Close, no t.Cleanup, no recoverable error.
//
// This test is unusual in that the OLD behaviour could not be observed as a
// failure. os.Exit(1) inside a test takes the whole binary down mid-run, so
// the regression shows up as the package aborting rather than as a red test.
// That is precisely why it is worth pinning: nothing here caught it before,
// and nothing could have while the call was log.Fatal.
func TestNewSubstrateConnectionReturnsErrors(t *testing.T) {
	// A path that cannot exec. go-plugin fails in client.Client(), the first
	// of the two paths.
	missing := filepath.Join(t.TempDir(), "no-such-plugin")

	conn, err := NewSubstrateConnection(ConnectWithCommand(missing))
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("NewSubstrateConnection succeeded against a nonexistent plugin path")
	}
	if conn != nil {
		t.Errorf("got a non-nil connection alongside an error: %#v", conn)
	}
	// The error has to name what failed to be actionable -- the whole reason
	// this beats a process exit is that a caller can report it.
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error does not name the plugin path it tried; got %q", err)
	}
}
