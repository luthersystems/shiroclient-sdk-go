package plugin

import (
	"errors"
	"fmt"
	"net/rpc"
	"strings"
)

// ErrFlowSnapshotsNotSupported is returned by PluginRPC.ImportFlowSnapshotsMock
// when the plugin cannot import defflow snapshots: its binary predates the
// method, its Substrate does not implement FlowSnapshotSubstrate, or the
// implementation returned an error matching ErrFlowSnapshotsNotSupported.
// Nothing was written.
var ErrFlowSnapshotsNotSupported = errors.New("substrate plugin does not support defflow snapshot import")

// FlowSnapshotSubstrate is implemented by a Substrate that can import defflow
// snapshots (format "defflow-snapshot/1", luthersystems/substrate#707) into a
// mock.  It is separate from Substrate so that adding it did not break
// implementations of that interface: PluginRPCServer type-asserts its Impl
// to FlowSnapshotSubstrate and answers "not supported" when the assertion
// fails.
//
// ImportFlowSnapshotsMock writes the records of snapshots into the ledger of
// the mock tag, byte for byte, in one write outside any transaction, and
// returns the member runs it restored.  It checks every snapshot before it
// writes, and writes nothing when one fails a check.  The phylum the runs
// are on must be installed first.  Return an error matching
// ErrFlowSnapshotsNotSupported when the substrate cannot import snapshots.
type FlowSnapshotSubstrate interface {
	ImportFlowSnapshotsMock(tag string, snapshots []FlowSnapshot) ([]ImportedFlowRun, error)
}

// FlowSnapshot is one snapshot file: one family's export.
type FlowSnapshot struct {
	// Name names the snapshot in errors, for example its file path.
	Name string
	// Data is the file's contents, JSON.
	Data []byte
}

// ImportedFlowRun is a member run an import restored, as its header names
// it.
type ImportedFlowRun struct {
	Flow        string
	RunID       string
	FlowVersion string
	Phylum      string
}

// ArgsImportFlowSnapshotsMock encodes the arguments to ImportFlowSnapshotsMock
type ArgsImportFlowSnapshotsMock struct {
	Tag       string
	Snapshots []FlowSnapshot
}

// RespImportFlowSnapshotsMock encodes the response from ImportFlowSnapshotsMock
type RespImportFlowSnapshotsMock struct {
	Err  *Error
	Runs []ImportedFlowRun
	// NotSupported reports that the plugin cannot import snapshots.
	NotSupported bool
}

var _ FlowSnapshotSubstrate = (*PluginRPC)(nil)

// ImportFlowSnapshotsMock forwards the call
func (g *PluginRPC) ImportFlowSnapshotsMock(tag string, snapshots []FlowSnapshot) ([]ImportedFlowRun, error) {
	var resp RespImportFlowSnapshotsMock
	err := g.client.Call("Plugin.ImportFlowSnapshotsMock", &ArgsImportFlowSnapshotsMock{Tag: tag, Snapshots: snapshots}, &resp)
	if err != nil {
		// A plugin binary that predates the method answers "can't find
		// method".
		var se rpc.ServerError
		if errors.As(err, &se) && strings.Contains(string(se), "can't find method") {
			return nil, fmt.Errorf("%w: the plugin has no ImportFlowSnapshotsMock (%s)", ErrFlowSnapshotsNotSupported, string(se))
		}
		return nil, err
	}
	switch {
	case resp.NotSupported && resp.Err != nil:
		return nil, fmt.Errorf("%w: %s", ErrFlowSnapshotsNotSupported, resp.Err.Diagnostic)
	case resp.NotSupported:
		return nil, ErrFlowSnapshotsNotSupported
	case resp.Err != nil:
		return nil, resp.Err
	}
	return resp.Runs, nil
}

// ImportFlowSnapshotsMock forwards the call
func (s *PluginRPCServer) ImportFlowSnapshotsMock(args *ArgsImportFlowSnapshotsMock, resp *RespImportFlowSnapshotsMock) error {
	fs, ok := s.Impl.(FlowSnapshotSubstrate)
	if !ok {
		*resp = RespImportFlowSnapshotsMock{
			Err:          &Error{Diagnostic: fmt.Sprintf("%T does not implement plugin.FlowSnapshotSubstrate", s.Impl)},
			NotSupported: true,
		}
		return nil
	}
	runs, err := fs.ImportFlowSnapshotsMock(args.Tag, args.Snapshots)
	if err != nil {
		*resp = RespImportFlowSnapshotsMock{Err: s.newError(err), NotSupported: errors.Is(err, ErrFlowSnapshotsNotSupported)}
		return nil
	}
	*resp = RespImportFlowSnapshotsMock{Runs: runs}
	return nil
}
