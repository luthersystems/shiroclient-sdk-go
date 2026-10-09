package plugin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFlowSnapshots is a Substrate that records the snapshots it is given.
// Only ImportFlowSnapshotsMock is used; the embedded nil Substrate panics on
// anything else.
type fakeFlowSnapshots struct {
	Substrate
	err      error
	gotTag   string
	gotSnaps []FlowSnapshot
	runs     []ImportedFlowRun
}

func (f *fakeFlowSnapshots) ImportFlowSnapshotsMock(tag string, snaps []FlowSnapshot) ([]ImportedFlowRun, error) {
	f.gotTag, f.gotSnaps = tag, snaps
	return f.runs, f.err
}

func TestPluginImportFlowSnapshotsRoundTrip(t *testing.T) {
	fake := &fakeFlowSnapshots{runs: []ImportedFlowRun{
		{Flow: "loan", RunID: "r1", FlowVersion: "1", Phylum: "v1"},
		{Flow: "loan", RunID: "r2", FlowVersion: "1", Phylum: "v1"},
	}}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})
	snaps := []FlowSnapshot{
		{Name: "dir/a.json", Data: []byte(`{"format":"defflow-snapshot/1"}`)},
		{Name: "dir/b.json", Data: []byte(`{}`)},
	}
	runs, err := client.ImportFlowSnapshotsMock("tag-1", snaps)
	require.NoError(t, err)
	assert.Equal(t, "tag-1", fake.gotTag)
	assert.Equal(t, snaps, fake.gotSnaps, "every snapshot reaches the substrate, in order")
	assert.Equal(t, fake.runs, runs)
}

func TestPluginImportFlowSnapshotsError(t *testing.T) {
	fake := &fakeFlowSnapshots{err: errors.New("run loan/r1: a parked row is missing")}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})
	_, err := client.ImportFlowSnapshotsMock("tag", []FlowSnapshot{{Name: "a.json"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a parked row is missing")
	assert.NotErrorIs(t, err, ErrFlowSnapshotsNotSupported)
}

func TestPluginImportFlowSnapshotsNotSupported(t *testing.T) {
	cases := map[string]interface{}{
		"impl without FlowSnapshotSubstrate":        &PluginRPCServer{Impl: batchOnly{}},
		"impl reports ErrFlowSnapshotsNotSupported": &PluginRPCServer{Impl: &fakeFlowSnapshots{err: ErrFlowSnapshotsNotSupported}},
		"plugin binary without the method":          oldPluginServer{},
	}
	for name, rcvr := range cases {
		t.Run(name, func(t *testing.T) {
			client := pipeClient(t, rcvr)
			_, err := client.ImportFlowSnapshotsMock("tag", []FlowSnapshot{{Name: "a.json"}})
			require.ErrorIs(t, err, ErrFlowSnapshotsNotSupported, "got %v", err)
		})
	}
}
