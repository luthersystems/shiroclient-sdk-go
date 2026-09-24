package private_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/private"
)

const batchCommitted = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":true,
   "result":[{"id":0,"error_level":0,"result":"ok","code":null,"message":null,"data":null},
             {"id":1,"error_level":0,"result":"ok","code":null,"message":null,"data":null}]},
 "$commit_tx_id":"tx-batch","$com_block_num":"12","$sim_block_num":"11"}`

// batchGateway records each CallBatch request body and answers with a
// committed two-request batch.
func batchGateway(t *testing.T) (shiroclient.ShiroClient, <-chan map[string]interface{}) {
	t.Helper()
	got := make(chan map[string]interface{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got <- req
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, batchCommitted)
	}))
	t.Cleanup(srv.Close)
	return shiroclient.NewRPC([]shiroclient.Config{
		shiroclient.WithEndpoint(srv.URL),
		shiroclient.WithHTTPClient(srv.Client()),
	}), got
}

func mxfRequest(t *testing.T, dsid private.DSID) *private.EncodeRequest {
	t.Helper()
	return &private.EncodeRequest{
		Message: map[string]interface{}{"name": string(dsid)},
		Transforms: []*private.Transform{{
			ContextPath: ".",
			Header:      &private.TransformHeader{ProfilePaths: []string{".name"}, PrivatePaths: []string{".name"}},
		}},
	}
}

func TestCallBatchPerRequestTransientMXF(t *testing.T) {
	client, got := batchGateway(t)
	alice := mxfRequest(t, "alice")
	bob := mxfRequest(t, "bob")
	aliceConfigs, err := private.WithTransientMXF(alice)
	require.NoError(t, err)
	bobConfigs, err := private.WithTransientMXF(bob)
	require.NoError(t, err)
	seed, err := private.WithSeed()
	require.NoError(t, err)

	_, err = shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "create_profile", Configs: aliceConfigs},
		{Method: "create_profile", Configs: bobConfigs},
	}, seed)
	require.NoError(t, err)

	params := (<-got)["params"].(map[string]interface{})
	reqs := params["requests"].([]interface{})
	require.Len(t, reqs, 2)
	for i, want := range []*private.EncodeRequest{alice, bob} {
		wantJSON, err := json.Marshal(want)
		require.NoError(t, err)
		transient := reqs[i].(map[string]interface{})["transient"].(map[string]interface{})
		assert.Equal(t, []string{"mxf"}, keys(transient), "request %d carries only its own mxf; its seed yields to the batch's", i)
		assert.Equal(t, hex.EncodeToString(wantJSON), transient["mxf"], "request %d", i)
	}
	assert.NotEqual(t, reqs[0].(map[string]interface{})["transient"], reqs[1].(map[string]interface{})["transient"])

	shared := params["transient"].(map[string]interface{})
	assert.Equal(t, []string{"csprng_seed_private"}, keys(shared), "the seed is shared by the transaction")
	seedHex, ok := shared["csprng_seed_private"].(string)
	require.True(t, ok)
	seedBytes, err := hex.DecodeString(seedHex)
	require.NoError(t, err)
	assert.Len(t, seedBytes, 32)
}

func TestCallBatchPerRequestSeedNeedsBatchSeed(t *testing.T) {
	client, _ := batchGateway(t)
	configs, err := private.WithTransientMXF(mxfRequest(t, "alice"))
	require.NoError(t, err)
	_, err = shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "create_profile", Configs: configs},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private.WithSeed()")
}

func TestCallBatchMXFNotShared(t *testing.T) {
	client, _ := batchGateway(t)
	configs, err := private.WithTransientMXF(mxfRequest(t, "alice"))
	require.NoError(t, err)
	_, err = shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "create_profile"}}, configs...)
	require.Error(t, err, "mxf data belongs to one request")
	assert.Contains(t, err.Error(), `"mxf"`)
}

func keys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
