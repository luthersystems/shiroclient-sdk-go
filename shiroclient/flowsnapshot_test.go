package shiroclient_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
)

func TestReadFlowSnapshotDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(`"b"`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`"a"`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o600))
	snaps, err := shiroclient.ReadFlowSnapshotDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []shiroclient.FlowSnapshot{
		{Name: filepath.Join(dir, "a.json"), Data: []byte(`"a"`)},
		{Name: filepath.Join(dir, "b.json"), Data: []byte(`"b"`)},
	}, snaps, "every *.json file, in name order")

	_, err = shiroclient.ReadFlowSnapshotDir(t.TempDir())
	require.ErrorContains(t, err, "holds no snapshot")

	// A glob metacharacter in the path is a plain character, and a
	// directory named *.json is not a snapshot.
	odd := filepath.Join(t.TempDir(), "run[1]*?")
	require.NoError(t, os.Mkdir(odd, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(odd, "a.json"), []byte(`"a"`), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(odd, "sub.json"), 0o700))
	snaps, err = shiroclient.ReadFlowSnapshotDir(odd)
	require.NoError(t, err)
	assert.Equal(t, []shiroclient.FlowSnapshot{{Name: filepath.Join(odd, "a.json"), Data: []byte(`"a"`)}}, snaps)
}

// A *.json symlink that points out of the directory is refused, not read.
func TestReadFlowSnapshotDirStaysInDir(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(outside, []byte(`"secret"`), 0o600))
	dir := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "a.json")))
	_, err := shiroclient.ReadFlowSnapshotDir(dir)
	require.ErrorContains(t, err, "escapes")
}

func TestImportFlowSnapshotsClientWithoutSupport(t *testing.T) {
	_, err := shiroclient.ImportFlowSnapshots(context.Background(), plainClient{}, []shiroclient.FlowSnapshot{{Name: "a.json"}})
	require.ErrorIs(t, err, shiroclient.ErrFlowSnapshotsNotSupported, "got %v", err)
	_, err = shiroclient.ImportFlowSnapshots(context.Background(), shiroclient.NewRPC(nil), nil)
	require.ErrorIs(t, err, shiroclient.ErrFlowSnapshotsNotSupported, "an RPC client never imports: got %v", err)
}

// TestImportFlowSnapshotsMock runs an import against the real mock plugin.
// A plugin that predates the import (luthersystems/substrate#707) answers
// ErrFlowSnapshotsNotSupported.  One that has it refuses a snapshot that is
// not one, and writes nothing.
func TestImportFlowSnapshotsMock(t *testing.T) {
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	_, err = shiroclient.ImportFlowSnapshots(context.Background(), client, []shiroclient.FlowSnapshot{
		{Name: "bad.json", Data: []byte(`{"format":"defflow-snapshot/0"}`)},
	})
	require.Error(t, err)
	if errors.Is(err, shiroclient.ErrFlowSnapshotsNotSupported) {
		assert.Contains(t, err.Error(), "substratehcp release", "the error says what to upgrade")
		return
	}
	assert.Contains(t, err.Error(), "bad.json")
	assert.Contains(t, err.Error(), "defflow-snapshot/1")
}
