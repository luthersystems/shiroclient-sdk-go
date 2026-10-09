package mock

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flowSnapshotFake is a plugin.Substrate that imports snapshots.  Only
// ImportFlowSnapshotsMock is used; the embedded nil Substrate panics on
// anything else.
type flowSnapshotFake struct {
	plugin.Substrate
	err      error
	gotTag   string
	gotSnaps []plugin.FlowSnapshot
	runs     []plugin.ImportedFlowRun
}

func (f *flowSnapshotFake) ImportFlowSnapshotsMock(tag string, snaps []plugin.FlowSnapshot) ([]plugin.ImportedFlowRun, error) {
	f.gotTag, f.gotSnaps = tag, snaps
	return f.runs, f.err
}

func TestMockImportFlowSnapshotsThroughPlugin(t *testing.T) {
	fake := &flowSnapshotFake{runs: []plugin.ImportedFlowRun{{Flow: "loan", RunID: "r1", FlowVersion: "1", Phylum: "v1"}}}
	c := &mockShiroClient{substrate: fake, tag: "tag-1"}
	runs, err := c.ImportFlowSnapshots(context.Background(), []types.FlowSnapshot{
		{Name: "a.json", Data: []byte("{}")},
		{Name: "b.json", Data: []byte("[]")},
	})
	require.NoError(t, err)
	assert.Equal(t, "tag-1", fake.gotTag)
	assert.Equal(t, []plugin.FlowSnapshot{{Name: "a.json", Data: []byte("{}")}, {Name: "b.json", Data: []byte("[]")}}, fake.gotSnaps)
	assert.Equal(t, []types.ImportedFlowRun{{Flow: "loan", RunID: "r1", FlowVersion: "1", Phylum: "v1"}}, runs)
}

func TestMockImportFlowSnapshotsError(t *testing.T) {
	fake := &flowSnapshotFake{err: errors.New("run loan/r1: broken link")}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	_, err := c.ImportFlowSnapshots(context.Background(), []types.FlowSnapshot{{Name: "a.json"}})
	require.ErrorContains(t, err, "broken link")
	assert.NotErrorIs(t, err, types.ErrFlowSnapshotsNotSupported)
}

func TestMockImportFlowSnapshotsNotSupported(t *testing.T) {
	for name, sub := range map[string]plugin.Substrate{
		"no FlowSnapshotSubstrate":     noBatch{},
		"ErrFlowSnapshotsNotSupported": &flowSnapshotFake{err: fmt.Errorf("wrapped: %w", plugin.ErrFlowSnapshotsNotSupported)},
	} {
		t.Run(name, func(t *testing.T) {
			c := &mockShiroClient{substrate: sub, tag: "t"}
			_, err := c.ImportFlowSnapshots(context.Background(), []types.FlowSnapshot{{Name: "a.json"}})
			require.ErrorIs(t, err, types.ErrFlowSnapshotsNotSupported, "got %v", err)
		})
	}
}

func TestMockImportFlowSnapshotsCancelled(t *testing.T) {
	fake := &flowSnapshotFake{}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.ImportFlowSnapshots(ctx, []types.FlowSnapshot{{Name: "a.json"}})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, fake.gotSnaps, "a cancelled import reaches the plugin")
}

func TestMockImportFlowSnapshotsTooLarge(t *testing.T) {
	fake := &flowSnapshotFake{}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	mib := make([]byte, 1<<20)
	snaps := make([]types.FlowSnapshot, types.MaxFlowSnapshotBytes>>20+1)
	for i := range snaps {
		snaps[i] = types.FlowSnapshot{Name: fmt.Sprintf("%d.json", i), Data: mib}
	}
	_, err := c.ImportFlowSnapshots(context.Background(), snaps)
	require.ErrorIs(t, err, types.ErrFlowSnapshotTooLarge)
	assert.Nil(t, fake.gotSnaps, "an import over the cap reaches the plugin")
	_, err = c.ImportFlowSnapshots(context.Background(), snaps[:len(snaps)-1])
	require.NoError(t, err, "an import at the cap")
}

