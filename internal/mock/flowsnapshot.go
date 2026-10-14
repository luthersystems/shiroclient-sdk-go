package mock

import (
	"context"
	"errors"
	"fmt"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
)

var _ types.FlowSnapshotImporter = (*mockShiroClient)(nil)

// ImportFlowSnapshots implements types.FlowSnapshotImporter through the
// plugin's FlowSnapshotSubstrate.  An older plugin yields
// types.ErrFlowSnapshotsNotSupported.  A ctx that has ended stops the import
// before it starts; the plugin call itself does not take ctx.
func (c *mockShiroClient) ImportFlowSnapshots(ctx context.Context, snapshots []types.FlowSnapshot) ([]types.ImportedFlowRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("import defflow snapshots: %w", err)
	}
	if err := types.CheckFlowSnapshotsSize(snapshots); err != nil {
		return nil, fmt.Errorf("import defflow snapshots: %w", err)
	}
	fs, ok := c.substrate.(plugin.FlowSnapshotSubstrate)
	if !ok {
		return nil, fmt.Errorf("%w: the mock substrate plugin cannot import defflow snapshots", types.ErrFlowSnapshotsNotSupported)
	}
	args := make([]plugin.FlowSnapshot, len(snapshots))
	for i, s := range snapshots {
		args[i] = plugin.FlowSnapshot{Name: s.Name, Data: s.Data}
	}
	runs, err := fs.ImportFlowSnapshotsMock(c.tag, args)
	if err != nil {
		if errors.Is(err, plugin.ErrFlowSnapshotsNotSupported) {
			return nil, fmt.Errorf("%w: the mock substrate plugin needs a substratehcp release that implements plugin.FlowSnapshotSubstrate: %w", types.ErrFlowSnapshotsNotSupported, err)
		}
		return nil, err
	}
	out := make([]types.ImportedFlowRun, len(runs))
	for i, r := range runs {
		out[i] = types.ImportedFlowRun{Flow: r.Flow, RunID: r.RunID, FlowVersion: r.FlowVersion, Phylum: r.Phylum}
	}
	return out, nil
}

// importPendingFlowSnapshots imports the snapshots of WithFlowSnapshots,
// once: Init calls it after the mock's first Init succeeds, and NewMock
// right after a restore.  It holds pendingMu through the import, so a
// concurrent Init returns only after the import ends.
func (c *mockShiroClient) importPendingFlowSnapshots(ctx context.Context) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	snaps := c.pendingFlowSnapshots
	c.pendingFlowSnapshots = nil
	if snaps == nil {
		return nil
	}
	if _, err := c.ImportFlowSnapshots(ctx, snaps); err != nil {
		return fmt.Errorf("mock.WithFlowSnapshots: import: %w", err)
	}
	return nil
}
