package types

import (
	"context"
	"errors"
)

// ErrFlowSnapshotsNotSupported is returned by ImportFlowSnapshots when the
// client cannot import defflow snapshots: it is not a mock, or its
// substratehcp plugin predates the import.  Nothing was written.
var ErrFlowSnapshotsNotSupported = errors.New("shiroclient: defflow snapshot import not supported")

// FlowSnapshot is one defflow snapshot file (format "defflow-snapshot/1"):
// one family's export, as <flow>_parked_export wrote it.
type FlowSnapshot struct {
	// Name names the snapshot in errors, for example its file path.
	Name string
	// Data is the file's contents.
	Data []byte
}

// ImportedFlowRun is a member run a snapshot import restored, as its header
// names it.
type ImportedFlowRun struct {
	Flow        string
	RunID       string
	FlowVersion string
	Phylum      string
}

// FlowSnapshotImporter is implemented by clients that can import defflow
// snapshots into their ledger: mock clients.  It is not part of ShiroClient.
type FlowSnapshotImporter interface {
	ImportFlowSnapshots(ctx context.Context, snapshots []FlowSnapshot) ([]ImportedFlowRun, error)
}
