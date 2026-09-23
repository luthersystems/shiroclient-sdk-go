package shiroclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/stretchr/testify/require"
)

// captureCallParams starts a gateway stub that records the "params" object of
// every JSON-RPC request it receives and answers each with an empty success.
func captureCallParams(t *testing.T) (string, <-chan map[string]interface{}) {
	t.Helper()
	got := make(chan map[string]interface{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req struct {
			Params map[string]interface{} `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got <- req.Params
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"1","result":{"error_level":0,"result":null,"code":null,"message":null,"data":null}}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

func TestWithoutTargetEndpointsSendsNotTargetEndpoints(t *testing.T) {
	url, got := captureCallParams(t)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})

	excluded := []string{"peer0.org1.example.com", "grpcs://peer1.org2.example.com:7051"}
	_, err := client.Call(context.Background(), "healthcheck",
		shiroclient.WithoutTargetEndpoints(excluded))
	require.NoError(t, err)

	params := <-got
	require.Equal(t, []interface{}{"peer0.org1.example.com", "grpcs://peer1.org2.example.com:7051"}, params["not_target_endpoints"])
	require.NotContains(t, params, "target_endpoints")
}

func TestNotTargetEndpointsOmittedByDefault(t *testing.T) {
	url, got := captureCallParams(t)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})

	_, err := client.Call(context.Background(), "healthcheck")
	require.NoError(t, err)
	params := <-got
	require.NotContains(t, params, "not_target_endpoints")
	require.NotContains(t, params, "target_endpoints")
}

func TestWithoutTargetEndpointsAsBaseConfig(t *testing.T) {
	url, got := captureCallParams(t)
	client := shiroclient.NewRPC([]shiroclient.Config{
		shiroclient.WithEndpoint(url),
		shiroclient.WithoutTargetEndpoints([]string{"peer-restoring"}),
	})

	_, err := client.Call(context.Background(), "healthcheck")
	require.NoError(t, err)
	require.Equal(t, []interface{}{"peer-restoring"}, (<-got)["not_target_endpoints"])
}
