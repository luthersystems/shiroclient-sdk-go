package update_test

import (
	"context"
	_ "embed"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/update"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
)

//go:embed shiroclient_test.lisp
var testPhylum []byte

const defaultPhylumID = "test"

func client(t *testing.T) shiroclient.ShiroClient {
	t.Helper()
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	err = client.Init(shiroclient.EncodePhylumBytes(testPhylum))
	require.NoError(t, err)
	return client
}

func TestGetPhyla(t *testing.T) {
	client := client(t)
	ctx := context.Background()

	phyla, err := update.GetPhyla(ctx, client)
	require.NoError(t, err)

	require.Len(t, phyla.Phyla, 1)
	require.Equal(t, phyla.Phyla[0].PhylumID, defaultPhylumID)
	require.Equal(t, phyla.Phyla[0].Status, update.StatusInService)
	require.NotEmpty(t, phyla.Phyla[0].Fingerprint)
	require.NotEmpty(t, phyla.Phyla[0].InitTimestamp)
}

func TestEnableDisable(t *testing.T) {
	client := client(t)
	ctx := context.Background()

	t.Run("enable-unknown-err", func(t *testing.T) {
		err := update.Enable(ctx, client, "unknown")
		assert.Error(t, err)
	})

	t.Run("enable-latest-err", func(t *testing.T) {
		err := update.Enable(ctx, client, "latest")
		assert.Error(t, err)
	})

	t.Run("enable", func(t *testing.T) {
		err := update.Enable(ctx, client, defaultPhylumID)
		assert.NoError(t, err)
	})

	t.Run("disable", func(t *testing.T) {
		err := update.Disable(ctx, client, defaultPhylumID)
		assert.NoError(t, err)
	})

	t.Run("re-enable", func(t *testing.T) {
		phyla, err := update.GetPhyla(ctx, client)
		require.NoError(t, err)
		require.Equal(t, phyla.Phyla[0].Status, update.StatusDisabled)

		err = update.Enable(ctx, client, defaultPhylumID)
		assert.NoError(t, err)

		phyla, err = update.GetPhyla(ctx, client)
		require.NoError(t, err)
		require.Equal(t, phyla.Phyla[0].Status, update.StatusInService)
	})
}

func call(ctx context.Context, client shiroclient.ShiroClient, method string, configs ...shiroclient.Config) ([]byte, error) {
	sr, err := client.Call(ctx, method, configs...)
	if err != nil {
		return nil, err
	}

	if sr.Error() != nil {
		return nil, errors.New(sr.Error().Message())
	}

	return sr.ResultJSON(), nil
}

func TestInstall(t *testing.T) {
	client := client(t)
	ctx := context.Background()

	t.Run("init-2", func(t *testing.T) {
		err := client.Init(shiroclient.EncodePhylumBytes(testPhylum), plugin.WithNewPhylumVersion("new"), shiroclient.WithContext(ctx))
		require.NoError(t, err)

		phyla, err := update.GetPhyla(ctx, client)
		require.NoError(t, err)
		require.Len(t, phyla.Phyla, 2)
	})

	t.Run("install", func(t *testing.T) {
		err := update.Install(ctx, client, "test2", testPhylum)
		require.NoError(t, err)

		phyla, err := update.GetPhyla(ctx, client)
		require.NoError(t, err)
		require.Len(t, phyla.Phyla, 3)
	})

	t.Run("withPhylum", func(t *testing.T) {
		resp, err := call(ctx, client, "read")
		require.NoError(t, err)
		require.Equal(t, `"test"`, string(resp))

		resp, err = call(ctx, client, "read", shiroclient.WithPhylumVersion("test2"))
		require.NoError(t, err)
		require.Equal(t, `"test2"`, string(resp))
	})
}
