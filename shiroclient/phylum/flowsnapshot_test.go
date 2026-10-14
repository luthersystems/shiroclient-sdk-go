package phylum_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/phylum"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func fmtPhylum(version string) string { return fmt.Sprintf(versionedPhylum, version) }

const versionedPhylum = `(in-package 'versioned)
(use-package 'router)
(defendpoint init () (route-success ()))
(defendpoint "which" (req) (route-success "%s"))`

// MockInstall puts a phylum version under its own label, and
// MockImportFlowSnapshots reaches the import; neither works on an RPC client.
func TestClientMockInstallAndImport(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	entry := logrus.NewEntry(log)
	client, err := phylum.NewMock(fmtPhylum("test"), entry)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	require.NoError(t, client.MockInstall(ctx, "1.0.0", []byte(fmtPhylum("1.0.0"))))
	for _, v := range []string{"test", "1.0.0"} {
		got, cerr := phylum.Call(client, ctx, "which", wrapperspb.String(""), &wrapperspb.StringValue{}, shiroclient.WithPhylumVersion(v))
		require.NoError(t, cerr)
		require.Equal(t, v, got.GetValue())
	}

	_, err = client.MockImportFlowSnapshots(ctx, []shiroclient.FlowSnapshot{{Name: "bad.json", Data: []byte(`{"format":"defflow-snapshot/0"}`)}})
	require.Error(t, err)
	if !errors.Is(err, shiroclient.ErrFlowSnapshotsNotSupported) {
		require.ErrorIs(t, err, shiroclient.ErrFlowSnapshotFormat)
	}

	rpc, err := phylum.New("http://127.0.0.1:1", entry)
	require.NoError(t, err)
	require.ErrorContains(t, rpc.MockInstall(ctx, "1.0.0", []byte("x")), "not a mock")
	_, err = rpc.MockImportFlowSnapshots(ctx, nil)
	require.ErrorIs(t, err, shiroclient.ErrFlowSnapshotsNotSupported)
}
