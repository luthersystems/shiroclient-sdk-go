package shiroclient

import (
	"context"
	"fmt"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
)

// FlowSnapshot is one defflow snapshot file (format "defflow-snapshot/1"):
// one family's export, as a phylum's <flow>_parked_export endpoint wrote it
// from a real ledger.
type FlowSnapshot = types.FlowSnapshot

// ImportedFlowRun is a member run a snapshot import restored, as its header
// names it.
type ImportedFlowRun = types.ImportedFlowRun

// FlowSnapshotImporter is implemented by clients that can import defflow
// snapshots; NewMock clients do.  It is not part of ShiroClient.
type FlowSnapshotImporter = types.FlowSnapshotImporter

// ErrFlowSnapshotsNotSupported matches, via errors.Is, an import that could
// not run at all: the client does not implement FlowSnapshotImporter (an
// RPC client), or it is a mock whose substratehcp plugin predates the import
// (luthersystems/substrate#707).  Nothing was written.
var ErrFlowSnapshotsNotSupported = types.ErrFlowSnapshotsNotSupported

// Refusals of an import, by kind: an import error matches at most one of
// them with errors.Is.  An error that matches none is a failure of the
// mock or the plugin, not of the snapshots.
var (
	ErrFlowSnapshotFormat       = types.ErrFlowSnapshotFormat
	ErrFlowSnapshotNotInstalled = types.ErrFlowSnapshotNotInstalled
	ErrFlowSnapshotConflict     = types.ErrFlowSnapshotConflict
	ErrFlowSnapshotInvalid      = types.ErrFlowSnapshotInvalid
	ErrFlowSnapshotTooLarge     = types.ErrFlowSnapshotTooLarge
)

// MaxFlowSnapshotBytes caps the total Data of one import.
const MaxFlowSnapshotBytes = types.MaxFlowSnapshotBytes

// ImportFlowSnapshots writes defflow snapshots into a mock client's ledger,
// so a Go test can load production runs and drive a migration with Call.  It
// returns the member runs it restored.
//
// The import is the one `shirotester flow-*` makes: it writes the records
// byte for byte, outside any transaction.  It checks every snapshot before
// it writes, and writes nothing when one fails: a format mismatch, a missing
// required record, a broken link between runs, or a record the ledger
// already holds with other content.  Install the phylum version the runs are
// on before the import, and the candidate version after it.
//
// Every snapshot goes to the plugin in one message, so one import holds at
// most MaxFlowSnapshotBytes of snapshot data (ErrFlowSnapshotTooLarge).
//
// A ctx that has ended stops the import before it starts.
//
// The import is test tooling: only mock clients support it.  Any other
// client, or a mock whose substratehcp plugin predates the import, returns
// an error matching ErrFlowSnapshotsNotSupported.
func ImportFlowSnapshots(ctx context.Context, client ShiroClient, snapshots []FlowSnapshot) ([]ImportedFlowRun, error) {
	fi, ok := client.(FlowSnapshotImporter)
	if !ok {
		return nil, fmt.Errorf("%w: %T does not implement FlowSnapshotImporter", ErrFlowSnapshotsNotSupported, client)
	}
	return fi.ImportFlowSnapshots(ctx, snapshots)
}

// ReadFlowSnapshotDir reads every *.json snapshot file of dir, in name
// order, as `shirotester flow-*` does.  A snapshot's Name is its path.  A
// directory that holds no *.json file is an error; a subdirectory is
// skipped.  Files are read through an os.Root of dir, so a symlink that
// points out of dir is an error, not a read.  Files whose total size
// exceeds MaxFlowSnapshotBytes are an error matching
// ErrFlowSnapshotTooLarge.
func ReadFlowSnapshotDir(dir string) ([]FlowSnapshot, error) {
	return types.ReadFlowSnapshotDir(dir)
}

// ImportFlowSnapshotDir is ImportFlowSnapshots of the snapshot files of dir
// (ReadFlowSnapshotDir).
func ImportFlowSnapshotDir(ctx context.Context, client ShiroClient, dir string) ([]ImportedFlowRun, error) {
	snaps, err := ReadFlowSnapshotDir(dir)
	if err != nil {
		return nil, err
	}
	return ImportFlowSnapshots(ctx, client, snaps)
}
