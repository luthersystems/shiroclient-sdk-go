package types

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrFlowSnapshotsNotSupported is returned by ImportFlowSnapshots when the
// client cannot import defflow snapshots: it is not a mock, or its
// substratehcp plugin predates the import.  Nothing was written.
var ErrFlowSnapshotsNotSupported = errors.New("shiroclient: defflow snapshot import not supported")

// Refusals of a snapshot import, by kind.  An import error matches at most
// one of them with errors.Is; its text says which record or run.
var (
	// ErrFlowSnapshotFormat: a snapshot is not valid JSON of format
	// "defflow-snapshot/1".
	ErrFlowSnapshotFormat = errors.New("defflow snapshot: bad format")
	// ErrFlowSnapshotNotInstalled: the ledger cannot run a run: no flow
	// route table, the run's flow is not installed, or the run's phylum
	// version is not installed at the run's flow version.
	ErrFlowSnapshotNotInstalled = errors.New("defflow snapshot: not installed")
	// ErrFlowSnapshotConflict: two snapshots, or a snapshot and the ledger,
	// hold different values of one record.
	ErrFlowSnapshotConflict = errors.New("defflow snapshot: conflicting record")
	// ErrFlowSnapshotInvalid: a run's records are not a complete, waiting,
	// closed family (a missing or unexpected record, a broken link).
	ErrFlowSnapshotInvalid = errors.New("defflow snapshot: invalid run")
	// ErrFlowSnapshotTooLarge: the snapshots of one import exceed
	// MaxFlowSnapshotBytes.
	ErrFlowSnapshotTooLarge = errors.New("defflow snapshot: import too large")
)

// MaxFlowSnapshotBytes caps the total Data of one import.  Every snapshot
// goes to the plugin in one RPC message, and gob refuses a message near
// 1 GiB.
const MaxFlowSnapshotBytes = 256 << 20

// CheckFlowSnapshotsSize returns an error matching ErrFlowSnapshotTooLarge
// when the snapshots' total Data exceeds MaxFlowSnapshotBytes.
func CheckFlowSnapshotsSize(snapshots []FlowSnapshot) error {
	n := 0
	for _, s := range snapshots {
		n += len(s.Data)
	}
	return checkFlowSnapshotBytes(n)
}

func checkFlowSnapshotBytes(n int) error {
	if n > MaxFlowSnapshotBytes {
		return fmt.Errorf("%w: %d bytes, over the cap of %d", ErrFlowSnapshotTooLarge, n, MaxFlowSnapshotBytes)
	}
	return nil
}

// ReadFlowSnapshotDir reads every *.json snapshot file of dir, in name
// order, as `shirotester flow-*` does.  A snapshot's Name is its path.  A
// directory that holds no *.json file is an error; a subdirectory is
// skipped.  Files are read through an os.Root of dir, so a symlink that
// points out of dir is an error, not a read.  Files whose total size
// exceeds MaxFlowSnapshotBytes are an error matching
// ErrFlowSnapshotTooLarge.
func ReadFlowSnapshotDir(dir string) ([]FlowSnapshot, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	entries, err := fs.ReadDir(root.FS(), ".") // sorted by name
	if err != nil {
		return nil, err
	}
	var out []FlowSnapshot
	total := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := root.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		total += len(b)
		if err := checkFlowSnapshotBytes(total); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		out = append(out, FlowSnapshot{Name: filepath.Join(dir, e.Name()), Data: b})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s holds no snapshot (*.json)", dir)
	}
	return out, nil
}

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
