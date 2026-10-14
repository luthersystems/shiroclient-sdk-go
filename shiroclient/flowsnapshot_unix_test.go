//go:build unix

package shiroclient_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A *.json entry that is not a regular file (a FIFO, a symlink to a
// directory) is skipped: reading a FIFO would block.
func TestReadFlowSnapshotDirSkipsNonRegular(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`"a"`), 0o600))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo.json"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.Symlink("sub", filepath.Join(dir, "link.json")))
	snaps, err := shiroclient.ReadFlowSnapshotDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []shiroclient.FlowSnapshot{{Name: filepath.Join(dir, "a.json"), Data: []byte(`"a"`)}}, snaps)
}

// A file over the cap is refused before it is read: an 8 GiB sparse file
// allocates nothing.
func TestReadFlowSnapshotDirRefusesHugeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.json")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Truncate(path, 8<<30))
	_, err := shiroclient.ReadFlowSnapshotDir(dir)
	require.ErrorIs(t, err, shiroclient.ErrFlowSnapshotTooLarge)
}
