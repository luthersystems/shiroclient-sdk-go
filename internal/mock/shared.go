package mock

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
)

// sharedKey identifies the plugin processes mocks may share: the same
// binary started with the same options. The writer is compared by identity.
type sharedKey struct {
	writer io.Writer
	path   string
	level  hclog.Level
}

// sharedConn is one pooled plugin process and the mocks it hosts.
type sharedConn struct {
	conn  *plugin.SubstrateConnection
	timer *time.Timer
	key   sharedKey
	refs  int
	idle  time.Duration
}

var sharedPool = struct {
	m  map[sharedKey]*sharedConn
	mu sync.Mutex
}{m: map[sharedKey]*sharedConn{}}

// canShare reports whether w can be part of a map key. A writer whose
// dynamic type is not comparable would panic there, so such mocks fall back
// to a process of their own.
func canShare(w io.Writer) bool {
	return w == nil || reflect.TypeOf(w).Comparable()
}

// acquireShared returns a pooled connection for key, starting a process
// with connect when there is none or the pooled one has exited. The caller
// must call releaseShared exactly once.
func acquireShared(key sharedKey, idle time.Duration, connect func() (*plugin.SubstrateConnection, error)) (*sharedConn, error) {
	sharedPool.mu.Lock()
	defer sharedPool.mu.Unlock()
	if sc := sharedPool.m[key]; sc != nil {
		if !sc.conn.Exited() {
			if sc.timer != nil {
				sc.timer.Stop()
				sc.timer = nil
			}
			sc.refs++
			sc.idle = idle
			return sc, nil
		}
		// The process died (for example it crashed); mocks still holding
		// it will see errors, new mocks get a fresh process.
		delete(sharedPool.m, key)
	}
	conn, err := connect()
	if err != nil {
		return nil, err
	}
	sc := &sharedConn{conn: conn, key: key, refs: 1, idle: idle}
	sharedPool.m[key] = sc
	return sc, nil
}

// releaseShared drops one mock's reference. The process stops when the
// last reference goes, after the idle timeout.
func releaseShared(sc *sharedConn) error {
	sharedPool.mu.Lock()
	defer sharedPool.mu.Unlock()
	sc.refs--
	if sc.refs > 0 {
		return nil
	}
	if sc.idle <= 0 {
		return sc.stopLocked()
	}
	sc.timer = time.AfterFunc(sc.idle, func() {
		sharedPool.mu.Lock()
		defer sharedPool.mu.Unlock()
		if sc.refs == 0 && sharedPool.m[sc.key] == sc {
			_ = sc.stopLocked()
		}
	})
	return nil
}

// stopLocked kills the process and removes it from the pool. The pool lock
// must be held.
func (sc *sharedConn) stopLocked() error {
	if sc.timer != nil {
		sc.timer.Stop()
		sc.timer = nil
	}
	if sharedPool.m[sc.key] == sc {
		delete(sharedPool.m, sc.key)
	}
	return sc.conn.Close()
}

// ShutdownSharedPlugins stops every shared plugin process now, including
// ones that still host open mocks (their later calls fail). Mocks created
// afterwards start new processes. It is safe to call at any time, for
// example from TestMain after m.Run.
func ShutdownSharedPlugins() error {
	sharedPool.mu.Lock()
	defer sharedPool.mu.Unlock()
	var errs []error
	for _, sc := range sharedPool.m {
		if err := sc.stopLocked(); err != nil {
			errs = append(errs, fmt.Errorf("stop plugin %q: %w", sc.key.path, err))
		}
	}
	return errors.Join(errs...)
}
