package shiroclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
// directory that holds no *.json file is an error.
func ReadFlowSnapshotDir(dir string) ([]FlowSnapshot, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s holds no snapshot (*.json)", dir)
	}
	sort.Strings(paths)
	out := make([]FlowSnapshot, len(paths))
	for i, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // a snapshot file in the directory the caller named
		if err != nil {
			return nil, err
		}
		out[i] = FlowSnapshot{Name: p, Data: b}
	}
	return out, nil
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
