package shiroclient_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
)

func newCreatorMock(t *testing.T, opts ...mock.Option) shiroclient.MockShiroClient {
	t.Helper()
	client, err := shiroclient.NewMock(nil, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	initClient(t, client, testPhylum)
	return client
}

func callCreator(t *testing.T, client shiroclient.ShiroClient, configs ...shiroclient.Config) (string, error) {
	t.Helper()
	sr, err := client.Call(context.Background(), "creator", configs...)
	require.NoError(t, err)
	if sr.Error() != nil {
		return "", sr.Error()
	}
	var msp string
	require.NoError(t, sr.UnmarshalTo(&msp))
	return msp, nil
}

// A new mock sets the fake creator MSP DefaultCreator, so cc:creator (and
// the MSP checks built on it) runs in memory without any setup.
func TestMockDefaultCreator(t *testing.T) {
	require.Equal(t, "Org1MSP", mock.DefaultCreator)
	client := newCreatorMock(t)
	msp, err := callCreator(t, client)
	require.NoError(t, err)
	require.Equal(t, mock.DefaultCreator, msp)
}

func TestMockWithCreator(t *testing.T) {
	client := newCreatorMock(t, mock.WithCreator("Org2MSP"))
	msp, err := callCreator(t, client)
	require.NoError(t, err)
	require.Equal(t, "Org2MSP", msp)
}

// WithCreator("") leaves the creator unset, so a phylum's creator read
// fails closed.
func TestMockWithoutCreatorFailsClosed(t *testing.T) {
	client := newCreatorMock(t, mock.WithCreator(""))
	_, err := callCreator(t, client)
	require.Error(t, err)

	// The failure came from the missing creator: setting one fixes the call.
	require.NoError(t, client.SetCreatorWithAttributes("Org2MSP", nil))
	msp, err := callCreator(t, client)
	require.NoError(t, err)
	require.Equal(t, "Org2MSP", msp)
}

// A snapshot does not carry the creator: a mock restored from one sets its
// own option's creator (DefaultCreator unless WithCreator says otherwise).
func TestMockCreatorAfterSnapshotRestore(t *testing.T) {
	src := newCreatorMock(t, mock.WithCreator("Org3MSP"))
	var snapshot bytes.Buffer
	require.NoError(t, src.Snapshot(&snapshot))

	for _, tc := range []struct {
		name string
		opts []mock.Option
		want string
	}{
		{"default", nil, mock.DefaultCreator},
		{"with creator", []mock.Option{mock.WithCreator("Org2MSP")}, "Org2MSP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]mock.Option{mock.WithSnapshotReader(bytes.NewReader(snapshot.Bytes()))}, tc.opts...)
			restored, err := shiroclient.NewMock(nil, opts...)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, restored.Close()) })
			msp, err := callCreator(t, restored)
			require.NoError(t, err)
			require.Equal(t, tc.want, msp)
		})
	}
}

// The per-call config still replaces the default creator.
func TestMockPerCallCreatorOverridesDefault(t *testing.T) {
	client := newCreatorMock(t)
	msp, err := callCreator(t, client, shiroclient.WithCreator("Org3MSP"))
	require.NoError(t, err)
	require.Equal(t, "Org3MSP", msp)
}

// Mocks sharing a plugin process each keep their own creator.
func TestMockSharedPluginCreatorPerMock(t *testing.T) {
	a := newCreatorMock(t, mock.WithSharedPlugin())
	b := newCreatorMock(t, mock.WithSharedPlugin(), mock.WithCreator("Org2MSP"))
	mspA, err := callCreator(t, a)
	require.NoError(t, err)
	mspB, err := callCreator(t, b)
	require.NoError(t, err)
	require.Equal(t, mock.DefaultCreator, mspA)
	require.Equal(t, "Org2MSP", mspB)
}
