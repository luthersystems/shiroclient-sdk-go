package private_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/private"
)

// batchCommitted is a committed batch of n successful requests.
func batchCommitted(n int) string {
	results := make([]string, n)
	for i := range results {
		results[i] = fmt.Sprintf(`{"id":%d,"error_level":0,"result":"ok","code":null,"message":null,"data":null}`, i)
	}
	return `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":true,
   "result":[` + strings.Join(results, ",") + `]},
 "$commit_tx_id":"tx-batch","$com_block_num":"12","$sim_block_num":"11"}`
}

// batchGateway records each CallBatch request body and answers with a
// committed batch of as many requests as it was sent.
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
		n := 0
		if params, ok := req["params"].(map[string]interface{}); ok {
			if reqs, ok := params["requests"].([]interface{}); ok {
				n = len(reqs)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, batchCommitted(n))
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

// seedOf returns the CSPRNG seed that configs set, as Call would send it.
func seedOf(t *testing.T, configs ...shiroclient.Config) string {
	t.Helper()
	seed, ok := types.ApplyConfigs(nil, configs...).Transient["csprng_seed_private"]
	require.True(t, ok, "configs carry a seed")
	return hex.EncodeToString(seed)
}

// sentSeed returns the batch's shared transient data and asserts no request
// carries a seed of its own.
func sentSeed(t *testing.T, got <-chan map[string]interface{}) map[string]interface{} {
	t.Helper()
	params := (<-got)["params"].(map[string]interface{})
	for i, r := range params["requests"].([]interface{}) {
		if transient, ok := r.(map[string]interface{})["transient"].(map[string]interface{}); ok {
			assert.NotContains(t, transient, "csprng_seed_private", "request %d sends no seed of its own", i)
		}
	}
	return params["transient"].(map[string]interface{})
}

func TestCallBatchPromotesFirstRequestSeed(t *testing.T) {
	for _, first := range []int{0, 2} {
		t.Run(fmt.Sprintf("request %d", first), func(t *testing.T) {
			client, got := batchGateway(t)
			requests := make([]shiroclient.CallBatchRequest, 4)
			var seeds []string
			for i := range requests {
				requests[i].Method = "create_profile"
				if i < first {
					continue // no seed before the first seeded request
				}
				configs, err := private.WithTransientMXF(mxfRequest(t, private.DSID(fmt.Sprint(i))))
				require.NoError(t, err)
				requests[i].Configs = configs
				seeds = append(seeds, seedOf(t, configs...))
			}
			require.NotEqual(t, seeds[0], seeds[1], "every WithSeed is fresh")

			_, err := shiroclient.CallBatch(context.Background(), client, requests)
			require.NoError(t, err)
			shared := sentSeed(t, got)
			assert.Equal(t, map[string]interface{}{"csprng_seed_private": seeds[0]}, shared,
				"the first seeded request's seed becomes the one transaction seed")
		})
	}
}

func TestCallBatchBatchSeedWins(t *testing.T) {
	client, got := batchGateway(t)
	aliceMXF, err := private.WithTransientMXF(mxfRequest(t, "alice"))
	require.NoError(t, err)
	bobMXF, err := private.WithTransientMXF(mxfRequest(t, "bob"))
	require.NoError(t, err)
	seed, err := private.WithSeed()
	require.NoError(t, err)

	_, err = shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "create_profile", Configs: aliceMXF},
		{Method: "create_profile", Configs: bobMXF},
	}, seed)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"csprng_seed_private": seedOf(t, seed)}, sentSeed(t, got),
		"the batch's own seed wins over every request's")
}

func TestCallBatchNoSeedIsNotRefused(t *testing.T) {
	client, got := batchGateway(t)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "create_profile", Configs: []shiroclient.Config{shiroclient.WithTransientData("mxf", []byte("{}"))}},
		{Method: "get"},
	})
	require.NoError(t, err, "a batch without a seed is sent; the server decides, as for Call")
	assert.Equal(t, map[string]interface{}{}, sentSeed(t, got))
}

func TestCallBatchPlainRequestSeedRefused(t *testing.T) {
	client, _ := batchGateway(t)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", Configs: []shiroclient.Config{shiroclient.WithTransientData("csprng_seed_private", []byte("s"))}},
	})
	require.Error(t, err, "only private.WithSeed's config is promotable")
	assert.Contains(t, err.Error(), "transaction-wide")
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
