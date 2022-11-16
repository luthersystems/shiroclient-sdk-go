package shiroclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"

	_ "embed"
)

//go:embed shiroclient_test.lisp
var testPhylum []byte

type healthcheckReport struct {
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version"`
	Status         string `json:"status"`
	Timestamp      string `json:"timestamp"`
}

type healthcheck struct {
	Reports []healthcheckReport `json:"reports"`
}

func call(client shiroclient.ShiroClient, method string, params interface{}, transient map[string][]byte) ([]byte, error) {
	sr, err := client.Call(context.Background(), method, shiroclient.WithParams(params), shiroclient.WithTransientDataMap(transient))
	if err != nil {
		return nil, err
	}

	if sr.Error() != nil {
		return nil, errors.New(sr.Error().Message())
	}

	return sr.ResultJSON(), nil
}

func initClient(t *testing.T, client shiroclient.ShiroClient) {
	err := client.Init(shiroclient.EncodePhylumBytes(testPhylum))
	require.NoError(t, err)
}

func TestHealth(t *testing.T) {
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	defer func() {
		err := client.Close()
		require.NoError(t, err)
	}()
	initClient(t, client)
	version, err := client.ShiroPhylum()
	require.NoError(t, err)
	require.Equal(t, "test", version)

	hcBytes, err := call(client, "healthcheck", map[string]interface{}{}, map[string][]byte{})
	require.NoError(t, err)

	fmt.Println(string(hcBytes))

	hc := &healthcheck{}
	err = json.Unmarshal(hcBytes, hc)
	require.NoError(t, err)

	if len(hc.Reports) != 1 {
		t.Fatalf("expected exactly one healthcheck report (got %d)", len(hc.Reports))
	}

	report := hc.Reports[0]
	require.Equal(t, "sample", report.ServiceName)
	require.Equal(t, "UP", report.Status)
}

func TestSnapshot(t *testing.T) {
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	initClient(t, client)

	storedVal := "sample"
	_, err = call(client, "write", []string{storedVal}, nil)
	require.NoError(t, err)

	var snapshot bytes.Buffer
	err = client.Snapshot(&snapshot)
	require.NoError(t, err)
	err = client.Close()
	require.NoError(t, err)

	r := bytes.NewReader(snapshot.Bytes())
	newClient, err := shiroclient.NewMock(nil, mock.WithSnapshotReader(r))
	require.NoError(t, err)
	defer func() {
		err := newClient.Close()
		require.NoError(t, err)
	}()

	resp, err := call(newClient, "read", map[string]interface{}{}, map[string][]byte{})
	require.NoError(t, err)
	var val string
	err = json.Unmarshal(resp, &val)
	require.NoError(t, err)
	require.Equal(t, storedVal, val)
}

func TestIsTimeoutError(t *testing.T) {
	var tests = []struct {
		msg string
		exp bool
	}{
		{"build/shirocore.lisp:730: lisp:error: GET_STATE failed: transaction ID: 2f82a776d9fdf67dca4caeef9d6b71bb1309d777c43bd45d091ab84587bfb9c6: no ledger context", true},
		{"transaction invalidated by the chaincode", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.exp, shiroclient.IsTimeoutError(errors.New(tt.msg)))
	}
}
