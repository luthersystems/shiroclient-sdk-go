package plugin

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestConnectWithLogOutput pins that the go-plugin CLIENT logger honours
// ConnectWithLogOutput, and that the default is unchanged.
//
// It asserts on the resolved connectOption rather than on NewSubstrateConnection,
// which would need a real plugin binary. That is the whole of the option's own
// contract; the one line beyond it -- handing this writer to hclog.New as
// Output -- is checked by writing through the logger built the same way.
//
// Why this exists: the writer was hardcoded to os.Stdout and only the LEVEL was
// reachable, so mock.WithLogWriter could not do what it documents. That gap was
// invisible precisely because nothing asserted it.
func TestConnectWithLogOutput(t *testing.T) {
	// The PRODUCTION constructor, not a replica of it. The previous version
	// rebuilt NewSubstrateConnection's literal here and said so ("Mirrors");
	// that asserts on the copy, so changing the real default would leave this
	// passing while describing a default that no longer exists.
	resolve := func(t *testing.T, opts ...ConnectOption) *connectOption {
		t.Helper()
		return newConnectOption(opts...)
	}

	t.Run("default resolves to os.Stdout through the logger", func(t *testing.T) {
		// Unset at the option layer -- newPluginLogger owns the fallback, so
		// that is where the default is observable.
		if got := resolve(t).logOutput; got != nil {
			t.Errorf("default logOutput = %v, want nil (newPluginLogger applies the default)", got)
		}
		assertLogsTo(t, resolve(t), wantStdout)
	})

	t.Run("option redirects the client logger", func(t *testing.T) {
		var buf bytes.Buffer
		co := resolve(t, ConnectWithLogOutput(&buf))
		if co.logOutput != &buf {
			t.Fatalf("logOutput = %v, want the supplied buffer", co.logOutput)
		}
		// Go through the REAL construction NewSubstrateConnection uses, not a
		// hand-built hclog: a test that rebuilt the logger itself would pass
		// even if the production path went back to hardcoding os.Stdout. A
		// mutation check caught exactly that, which is why newPluginLogger
		// exists.
		newPluginLogger(co).Error("plugin process exited")
		if !bytes.Contains(buf.Bytes(), []byte("plugin process exited")) {
			t.Errorf("client logger output did not reach the supplied writer; got %q", buf.String())
		}
	})

	t.Run("nil writer falls back to stdout rather than reaching hclog", func(t *testing.T) {
		// A nil Output makes hclog fall back to DefaultOutput, its init-time
		// snapshot of os.Stderr -- so the line escapes to the real process
		// stderr and NO caller redirection can catch it. newPluginLogger
		// guards against that by defaulting to os.Stdout instead.
		//
		// The previous version of this asserted `newPluginLogger(co) != nil`.
		// hclog.New never returns nil, so that could not fail: delete the
		// guard and it still passed, while its comment claimed to pin it.
		// This asserts on where the bytes go, and fails when the guard goes.
		co := resolve(t, ConnectWithLogOutput(nil))
		if co.logOutput != nil {
			t.Fatalf("expected the option to record nil, got %v", co.logOutput)
		}
		assertLogsTo(t, co, wantStdout)
	})

	t.Run("log output is independent of attachStdamp", func(t *testing.T) {
		var logBuf, stdioBuf bytes.Buffer
		co := resolve(t, ConnectWithLogOutput(&logBuf), ConnectWithAttachStdamp(&stdioBuf))
		if co.logOutput != &logBuf {
			t.Errorf("logOutput = %v, want the log buffer", co.logOutput)
		}
		if co.attachStdamp != &stdioBuf {
			t.Errorf("attachStdamp = %v, want the stdio buffer", co.attachStdamp)
		}
	})
}

// stream names the descriptor a log line is expected on.
type stream int

const (
	wantStdout stream = iota
)

// assertLogsTo rebinds BOTH os.Stdout and os.Stderr to pipes, logs one line
// through the production newPluginLogger, and asserts it arrived on the
// expected one and NOT the other.
//
// Watching where the bytes land is the only option: hclog does not expose its
// writer.
//
// The stdout assertion is the load-bearing one, and the reason is worse than
// "nil would go to stderr instead". hclog's nil fallback is `DefaultOutput`
// (logger.go: `DefaultOutput io.Writer = os.Stderr`) -- a package-level
// variable initialised when hclog is initialised. It is a SNAPSHOT of
// os.Stderr, not a read of it, so a nil Output escapes to the real process
// stderr and rebinding os.Stderr afterwards cannot intercept it. Verified: with
// the guard removed this helper sees stdout="" AND stderr="", because the line
// went somewhere neither pipe can reach.
//
// So the stderr check below is a secondary guard against hclog changing that
// behaviour; it is the MISSING stdout line that catches the guard's removal
// today.
func assertLogsTo(t *testing.T, co *connectOption, want stream) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	restore := func() { os.Stdout, os.Stderr = savedOut, savedErr }
	defer restore()

	outCh, errCh := make(chan string, 1), make(chan string, 1)
	go func() { b, _ := io.ReadAll(outR); outCh <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errCh <- string(b) }()

	const marker = "assertLogsTo probe line"
	newPluginLogger(co).Error(marker)

	restore()
	_ = outW.Close()
	_ = errW.Close()
	gotOut, gotErr := <-outCh, <-errCh
	_ = outR.Close()
	_ = errR.Close()

	if want != wantStdout {
		t.Fatalf("unsupported stream %d", want)
	}
	if !strings.Contains(gotOut, marker) {
		t.Errorf("log line did not reach os.Stdout; stdout=%q stderr=%q", gotOut, gotErr)
	}
	if strings.Contains(gotErr, marker) {
		t.Errorf("log line reached os.STDERR -- the nil-writer guard in "+
			"newPluginLogger is missing, so hclog substituted stderr; stderr=%q", gotErr)
	}
}
