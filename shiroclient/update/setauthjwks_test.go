package update_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/update"
)

// gatewayStub answers every JSON-RPC call with result and records the
// "params" object of each request it receives.
func gatewayStub(t *testing.T, result string) (string, <-chan map[string]json.RawMessage) {
	t.Helper()
	got := make(chan map[string]json.RawMessage, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req struct {
			Params map[string]json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got <- req.Params
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"1","result":`+result+`}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

const testJWKS = `{"keys":[{"crv":"Ed25519","kid":"k1","kty":"OKP","x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"}]}`

// TestSetAuthJWKS pins the request SetAuthJWKS sends (the $auth_set_jwks
// method, with the JWKS as the named parameter "jwks") and the key IDs it
// reads from the answer.  substrate's endpoint is new, so the mock plugin
// this module pins does not have it; the test stubs the gateway instead.
func TestSetAuthJWKS(t *testing.T) {
	url, got := gatewayStub(t, `{"error_level":0,"result":{"key_ids":["k1"]},"code":null,"message":null,"data":null}`)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})
	ids, err := update.SetAuthJWKS(context.Background(), client, []byte(testJWKS))
	require.NoError(t, err)
	require.Equal(t, []string{"k1"}, ids)

	params := <-got
	require.JSONEq(t, `"$auth_set_jwks"`, string(params["method"]))
	require.JSONEq(t, `{"jwks":`+testJWKS+`}`, string(params["params"]))
}

// TestSetAuthJWKSRefused pins that a refusal returns an error holding
// substrate's reason.
func TestSetAuthJWKSRefused(t *testing.T) {
	url, _ := gatewayStub(t, `{"error_level":2,"result":null,"code":-32000,"message":"Server error","data":"creator MSP Org2MSP may not set the auth JWKS"}`)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})
	_, err := update.SetAuthJWKS(context.Background(), client, []byte(testJWKS))
	require.EqualError(t, err, `Server error: "creator MSP Org2MSP may not set the auth JWKS"`)
}

// TestSetAuthJWKSNotJSON pins that a JWKS that is not JSON fails before
// any call.
func TestSetAuthJWKSNotJSON(t *testing.T) {
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint("http://127.0.0.1:0")})
	_, err := update.SetAuthJWKS(context.Background(), client, []byte("not json"))
	require.EqualError(t, err, "jwks is not a JSON document")
}
