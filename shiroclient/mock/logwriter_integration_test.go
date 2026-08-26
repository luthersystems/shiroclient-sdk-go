package mock_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
)

// syncBuf is an io.Writer safe for concurrent use. go-plugin logs from its own
// goroutines, so an unguarded bytes.Buffer would race under -race.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestWithLogWriterCapturesClientLogger is the end-to-end guard for
// WithLogWriter's documented promise: "sets the plugin's log destination".
//
// It did not keep that promise. WithLogWriter fed only the plugin
// SUBPROCESS's stdio, while go-plugin's host-side client logger was
// hardcoded to os.Stdout, so a caller passing a writer here still had
// "starting plugin" and every error-level line land on stdout with no way to
// stop them.
//
// The unit test in x/plugin covers the option and the logger construction.
// This covers the WIRING BETWEEN THEM -- internal/mock passing LogWriter to
// ConnectWithLogOutput -- which nothing else does: deleting that line still
// compiles and still passes every other test in the repo. A mutation check
// confirmed exactly that, which is why this test exists.
//
// Debug level is required: the line this asserts on ("starting plugin") is
// emitted by the go-plugin client at Debug.
func TestWithLogWriterCapturesClientLogger(t *testing.T) {
	var logs syncBuf

	client, err := shiroclient.NewMock(nil,
		mock.WithLogWriter(&logs),
		mock.WithLogLevel(mock.Debug),
	)
	if err != nil {
		t.Fatalf("NewMock: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	got := logs.String()
	if got == "" {
		t.Fatal("WithLogWriter received nothing at all")
	}
	// THE DISCRIMINATOR MATTERS. Both streams land in this one writer, so a
	// loose assertion passes on the subprocess alone and proves nothing --
	// the first version of this test did exactly that and survived the
	// mutation it was written to catch.
	//
	// The two are distinguishable by format. The host-side client logger is
	// the hclog built in newPluginLogger with Name "plugin", so its lines are
	// TEXT and carry a bracketed level plus that name:
	//
	//     2026-...Z [DEBUG] plugin: starting plugin: path=...
	//
	// The plugin SUBPROCESS logs JSON through attachStdamp:
	//
	//     {"@level":"debug","@message":"plugin address",...}
	//
	// With ConnectWithLogOutput removed, the buffer contains only the JSON.
	// So assert on the hclog text signature, which only the client logger
	// produces.
	if !strings.Contains(got, "[DEBUG] plugin:") {
		t.Errorf("no host-side go-plugin client logger output reached WithLogWriter's "+
			"writer -- it is still going elsewhere (hardcoded to os.Stdout before "+
			"this fix). Captured:\n%s", got)
	}
}
