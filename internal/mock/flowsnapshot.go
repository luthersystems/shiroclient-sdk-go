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
// types.ErrFlowSnapshotsNotSupported.  The plugin call does not take ctx.
func (c *mockShiroClient) ImportFlowSnapshots(_ context.Context, snapshots []types.FlowSnapshot) ([]types.ImportedFlowRun, error) {
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
