//go:build unix

package mock

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "embed"

	"github.com/luthersystems/shiroclient-sdk-go/internal/mockint"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
	"github.com/stretchr/testify/require"
)

//go:embed shared_test.lisp
var sharedTestPhylum []byte

func requirePlugin(t testing.TB) {
	t.Helper()
	if os.Getenv(mockint.DefaultPluginEnv) == "" {
		t.Skipf("%s not set", mockint.DefaultPluginEnv)
	}
}

func newTestMock(t testing.TB, opts ...mock.Option) *mockShiroClient {
	t.Helper()
	opts = append([]mock.Option{mock.WithLogWriter(io.Discard), mock.WithLogLevel(mock.Error)}, opts...)
	c, err := NewMock(nil, opts...)
	require.NoError(t, err)
	mc := c.(*mockShiroClient)
	return mc
}

func initMock(t testing.TB, c types.ShiroClient) {
	t.Helper()
	require.NoError(t, c.Init(context.Background(), base64.StdEncoding.EncodeToString(sharedTestPhylum)))
}

func callMock(t testing.TB, c types.ShiroClient, method string, params interface{}) string {
	t.Helper()
	opt := types.Opt(func(r *types.RequestOptions) { r.Params = params })
	resp, err := c.Call(context.Background(), method, opt)
	require.NoError(t, err)
	require.Nil(t, resp.Error())
	var out string
	if len(resp.ResultJSON()) > 0 && string(resp.ResultJSON()) != "null" {
		require.NoError(t, json.Unmarshal(resp.ResultJSON(), &out))
	}
	return out
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func waitDead(t testing.TB, pid int) {
	t.Helper()
	require.Eventually(t, func() bool { return !processAlive(pid) }, 10*time.Second, 20*time.Millisecond,
		"plugin process %d still running", pid)
}

// TestSharedPlugin_IsolationAndClose proves mocks hosted by one plugin
// process keep separate ledgers, and that closing one leaves the other
// working.
func TestSharedPlugin_IsolationAndClose(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	a := newTestMock(t, mock.WithSharedPlugin())
	b := newTestMock(t, mock.WithSharedPlugin())
	require.Same(t, a.conn, b.conn, "shared mocks must use one connection")
	require.NotEqual(t, a.tag, b.tag)
	pid := a.conn.Pid()
	require.True(t, processAlive(pid))

	initMock(t, a)
	initMock(t, b)
	callMock(t, a, "write", []interface{}{"from-a"})
	callMock(t, b, "write", []interface{}{"from-b"})
	require.Equal(t, "from-a", callMock(t, a, "read", nil))
	require.Equal(t, "from-b", callMock(t, b, "read", nil))

	// A snapshot restored into a fresh shared mock carries only its source.
	var snap bytes.Buffer
	require.NoError(t, a.Snapshot(&snap))
	c := newTestMock(t, mock.WithSharedPlugin(), mock.WithSnapshotReader(&snap))
	require.Same(t, a.conn, c.conn)
	require.Equal(t, "from-a", callMock(t, c, "read", nil))
	callMock(t, c, "write", []interface{}{"from-c"})
	require.Equal(t, "from-a", callMock(t, a, "read", nil))

	require.NoError(t, a.Close())
	require.True(t, processAlive(pid), "closing one mock must not stop the shared process")
	require.Equal(t, "from-b", callMock(t, b, "read", nil))
	require.Equal(t, "from-c", callMock(t, c, "read", nil))
	require.NoError(t, a.Close(), "a second Close is a no-op")

	require.NoError(t, b.Close())
	require.NoError(t, c.Close())
}

// TestSharedPlugin_ExitsAfterLastClose checks the reference count: with a
// zero idle timeout the process exits as soon as the last mock closes, and a
// later mock starts a new one.
func TestSharedPlugin_ExitsAfterLastClose(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	a := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(0))
	b := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(0))
	pid := a.conn.Pid()
	require.NoError(t, a.Close())
	require.True(t, processAlive(pid))
	require.NoError(t, b.Close())
	waitDead(t, pid)

	c := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(0))
	require.NotEqual(t, pid, c.conn.Pid())
	initMock(t, c)
	require.NoError(t, c.Close())
}

// TestSharedPlugin_IdleReuseAndShutdown checks that an idle process is
// reused within the idle timeout and stopped by ShutdownSharedPlugins.
func TestSharedPlugin_IdleReuseAndShutdown(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	a := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(time.Minute))
	pid := a.conn.Pid()
	require.NoError(t, a.Close())
	require.True(t, processAlive(pid), "idle process should linger")

	b := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(time.Minute))
	require.Equal(t, pid, b.conn.Pid(), "idle process should be reused")
	require.NoError(t, b.Close())

	require.NoError(t, ShutdownSharedPlugins())
	waitDead(t, pid)
}

// TestSharedPlugin_SeparateOptionSets checks that a different log level
// (part of the pool key) gets its own process.
func TestSharedPlugin_SeparateOptionSets(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	a := newTestMock(t, mock.WithSharedPlugin())
	b := newTestMock(t, mock.WithSharedPlugin(), mock.WithLogLevel(mock.Warn))
	require.NotSame(t, a.conn, b.conn)
	require.NoError(t, a.Close())
	require.NoError(t, b.Close())
}

// TestSharedPlugin_Concurrent runs many mocks on one process concurrently;
// run with -race.
func TestSharedPlugin_Concurrent(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	const n = 8
	var wg sync.WaitGroup
	pids := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := newTestMock(t, mock.WithSharedPlugin())
			defer func() { require.NoError(t, c.Close()) }()
			pids[i] = c.conn.Pid()
			initMock(t, c)
			v := fmt.Sprintf("value-%d", i)
			for j := 0; j < 3; j++ {
				callMock(t, c, "write", []interface{}{v})
				require.Equal(t, v, callMock(t, c, "read", nil))
			}
		}(i)
	}
	wg.Wait()
	for _, p := range pids {
		require.Equal(t, pids[0], p)
	}
}

// TestPerProcessMode_Unchanged checks the default still gives each mock its
// own process, stopped by Close.
func TestPerProcessMode_Unchanged(t *testing.T) {
	requirePlugin(t)
	a := newTestMock(t)
	b := newTestMock(t)
	require.NotEqual(t, a.conn.Pid(), b.conn.Pid())
	initMock(t, a)
	callMock(t, a, "write", []interface{}{"x"})
	pid := a.conn.Pid()
	require.NoError(t, a.Close())
	waitDead(t, pid)
	require.NoError(t, b.Close())
}

// BenchmarkRestoreCycle measures { new mock from snapshot; one call; Close }
// per mock, in each mode. Beyond ns/op it reports cpu-ms/op (user+system
// time of the test process and its reaped plugin processes) and, on Linux,
// peak-rss-MB: the peak of the summed RSS of the test process and its
// plugin processes, sampled every 5ms. The parallel8 variants run the
// cycles from 8 goroutines at once, as a parallel test suite would; the held8 variants keep 8 mocks open
// at once before closing them, as a test holding several fixtures would. held32 with -benchtime=32x
// is a single burst of 32 open mocks. Run
// with -benchtime=50x.
func BenchmarkRestoreCycle(b *testing.B) {
	requirePlugin(b)
	seed := newTestMock(b)
	initMock(b, seed)
	callMock(b, seed, "write", []interface{}{"seed"})
	var snap bytes.Buffer
	require.NoError(b, seed.Snapshot(&snap))
	require.NoError(b, seed.Close())

	restoreOpts := func(extra []mock.Option) []mock.Option {
		return append([]mock.Option{mock.WithSnapshotReader(bytes.NewReader(snap.Bytes()))}, extra...)
	}
	shared := []mock.Option{mock.WithSharedPlugin()}
	for _, mode := range []struct {
		name    string
		opts    []mock.Option
		workers int
		held    int
	}{
		{"per-process", nil, 1, 0},
		{"shared", shared, 1, 0},
		{"per-process-parallel8", nil, 8, 0},
		{"shared-parallel8", shared, 8, 0},
		{"per-process-held8", nil, 0, 8},
		{"shared-held8", shared, 0, 8},
		{"per-process-held32", nil, 0, 32},
		{"shared-held32", shared, 0, 32},
	} {
		b.Run(mode.name, func(b *testing.B) {
			runtime.GC()
			stop := sampleRSS()
			cpu0 := cpuTime()
			b.ResetTimer()
			if mode.held > 0 {
				for i := 0; i < b.N; i += mode.held {
					var open []*mockShiroClient
					for j := i; j < b.N && j < i+mode.held; j++ {
						c := newTestMock(b, restoreOpts(mode.opts)...)
						if got := callMock(b, c, "read", nil); got != "seed" {
							b.Errorf("read %q", got)
						}
						open = append(open, c)
					}
					for _, c := range open {
						require.NoError(b, c.Close())
					}
				}
			}
			var wg sync.WaitGroup
			for w := 0; w < mode.workers; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for i := w; i < b.N; i += mode.workers {
						c := newTestMock(b, restoreOpts(mode.opts)...)
						if got := callMock(b, c, "read", nil); got != "seed" {
							b.Errorf("read %q", got)
						}
						require.NoError(b, c.Close())
					}
				}(w)
			}
			wg.Wait()
			// Stop (and reap) the shared process inside the measurement so
			// its CPU is counted, as a per-process mock's is at Close.
			require.NoError(b, ShutdownSharedPlugins())
			b.StopTimer()
			waitChildrenReaped()
			b.ReportMetric(float64((cpuTime()-cpu0).Milliseconds())/float64(b.N), "cpu-ms/op")
			if peak := stop(); peak > 0 {
				b.ReportMetric(float64(peak)/(1<<20), "peak-rss-MB")
			}
		})
	}
}

func cpuTime() time.Duration {
	var self, kids syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &self)
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &kids)
	tv := func(t syscall.Timeval) time.Duration { return time.Duration(t.Nano()) }
	return tv(self.Utime) + tv(self.Stime) + tv(kids.Utime) + tv(kids.Stime)
}

// childPIDs lists the live child processes of the test process.
func childPIDs() []string {
	tasks, _ := filepath.Glob("/proc/self/task/*/children")
	var out []string
	for _, t := range tasks {
		b, err := os.ReadFile(t)
		if err == nil {
			out = append(out, strings.Fields(string(b))...)
		}
	}
	return out
}

func waitChildrenReaped() {
	deadline := time.Now().Add(10 * time.Second)
	for len(childPIDs()) > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func rssOf(pid string) int64 {
	b, err := os.ReadFile("/proc/" + pid + "/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				kb, _ := strconv.ParseInt(f[1], 10, 64)
				return kb << 10
			}
		}
	}
	return 0
}

// sampleRSS samples the summed RSS of this process and its children until
// the returned function is called, which returns the peak in bytes (0 when
// /proc is unavailable).
func sampleRSS() func() int64 {
	done := make(chan struct{})
	result := make(chan int64)
	go func() {
		var peak int64
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			sum := rssOf("self")
			for _, pid := range childPIDs() {
				sum += rssOf(pid)
			}
			if sum > peak {
				peak = sum
			}
			select {
			case <-done:
				result <- peak
				return
			case <-tick.C:
			}
		}
	}()
	return func() int64 { close(done); return <-result }
}

// TestSharedPlugin_Stress runs many shared mocks on one process at once,
// mixing init, writes, reads and snapshot/restore, and races Close of the
// last mock against a new mock's startup.  Run it with -race, and with a
// race-enabled plugin (GORACE=log_path=...) to check the plugin side too.
func TestSharedPlugin_Stress(t *testing.T) {
	requirePlugin(t)
	if testing.Short() {
		t.Skip("stress test")
	}
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })

	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := newTestMock(t, mock.WithSharedPlugin())
			defer func() { require.NoError(t, c.Close()) }()
			initMock(t, c)
			for j := 0; j < 4; j++ {
				v := fmt.Sprintf("value-%d-%d", i, j)
				callMock(t, c, "write", []interface{}{v})
				require.Equal(t, v, callMock(t, c, "read", nil))
				if j%2 == 1 {
					var snap bytes.Buffer
					require.NoError(t, c.Snapshot(&snap))
					r := newTestMock(t, mock.WithSharedPlugin(), mock.WithSnapshotReader(&snap))
					require.Equal(t, v, callMock(t, r, "read", nil))
					callMock(t, r, "write", []interface{}{"restored"})
					require.Equal(t, v, callMock(t, c, "read", nil))
					require.NoError(t, r.Close())
				}
			}
		}(i)
	}
	wg.Wait()

	// Close of the last mock (idle timeout 0 stops the process) racing a
	// new mock's startup on the same pool key.
	for k := 0; k < 8; k++ {
		old := newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(0))
		var cwg sync.WaitGroup
		cwg.Add(2)
		go func() { defer cwg.Done(); require.NoError(t, old.Close()) }()
		var fresh *mockShiroClient
		go func() {
			defer cwg.Done()
			fresh = newTestMock(t, mock.WithSharedPlugin(), mock.WithSharedPluginIdleTimeout(0))
		}()
		cwg.Wait()
		initMock(t, fresh)
		callMock(t, fresh, "write", []interface{}{"fresh"})
		require.Equal(t, "fresh", callMock(t, fresh, "read", nil))
		require.NoError(t, fresh.Close())
	}
}
