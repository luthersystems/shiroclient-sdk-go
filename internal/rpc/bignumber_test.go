package rpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// big is 2^53 + 1, the smallest positive integer a float64 cannot hold.
// A client that decodes it as a float64 returns 9007199254740992 instead.
// The gateway passes such numbers through unchanged
// (luthersystems/substrate#822), so the client must too
// (luthersystems/shiroclient-sdk-go#102).
const big = "9007199254740993"

// bigServer answers every request with body and records the last request
// body, so a test can check what the client sent.
func bigServer(t *testing.T, body string) (types.ShiroClient, *[]byte) {
	t.Helper()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	client := NewRPC([]types.Config{types.Opt(func(r *types.RequestOptions) { r.Endpoint, r.HTTPClient = srv.URL, srv.Client() })})
	return client, &got
}

func TestCallKeepsLargeIntegers(t *testing.T) {
	t.Run("result", func(t *testing.T) {
		client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":0,"result":{"n":`+big+`,"f":1.5,"list":[`+big+`]},"code":null,"message":null,"data":null}}`)
		res, err := client.Call(context.Background(), "test")
		require.NoError(t, err)
		assert.JSONEq(t, `{"n":`+big+`,"f":1.5,"list":[`+big+`]}`, string(res.ResultJSON()))
		assert.Contains(t, string(res.ResultJSON()), big)
	})
	t.Run("error data", func(t *testing.T) {
		client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":2,"result":null,"code":1000,"message":"bad","data":{"n":`+big+`}}}`)
		res, err := client.Call(context.Background(), "test")
		require.NoError(t, err)
		require.NotNil(t, res.Error())
		assert.Equal(t, 1000, res.Error().Code())
		assert.Contains(t, string(res.Error().DataJSON()), big)
	})
}

func TestInitKeepsLargeIntegers(t *testing.T) {
	client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":0,"result":`+big+`,"code":null,"message":null,"data":null}}`)
	var got []byte
	err := client.Init(context.Background(), "phylum", types.Opt(func(r *types.RequestOptions) {
		r.ResponseReceiver = func(res types.ShiroResponse) { got = res.ResultJSON() }
	}))
	require.NoError(t, err)
	assert.Equal(t, big, string(got))
}

func TestCallBatchKeepsLargeIntegers(t *testing.T) {
	client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":2,"committed":false,"result":[`+
		`{"id":0,"error_level":0,"result":{"n":`+big+`},"code":null,"message":null,"data":null},`+
		`{"id":1,"error_level":2,"result":null,"code":1000,"message":"bad","data":{"n":`+big+`}}`+
		`],"code":1000,"message":"bad","data":{"failed_index":1}}}`)
	br, err := client.(types.CallBatcher).CallBatch(context.Background(), []types.CallBatchRequest{{Method: "a"}, {Method: "b"}})
	var batchErr *types.CallBatchError
	require.ErrorAs(t, err, &batchErr)
	assert.Equal(t, 1, batchErr.Index)
	require.NotNil(t, br)
	assert.Contains(t, string(br.Responses[0].ResultJSON()), big)
	assert.Contains(t, string(br.Responses[1].Error().DataJSON()), big)
	// IDs stay float64, as CallBatchResponse.IDs documents and the mock
	// client returns.
	assert.Equal(t, []interface{}{float64(0), float64(1)}, br.IDs)
}

func TestQueryInfoKeepsLargeIntegers(t *testing.T) {
	client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":0,"result":`+big+`,"code":null,"message":null,"data":null}}`)
	height, err := client.QueryInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(9007199254740993), height)
}

func TestQueryBlockSendsExactBlockNumber(t *testing.T) {
	client, sent := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":0,"result":{"block_hash":"h","transaction_ids":[],"transaction_reasons":[],"transaction_events":[],"chaincode_ids":[]},"code":null,"message":null,"data":null}}`)
	_, err := client.QueryBlock(context.Background(), 9007199254740993)
	require.NoError(t, err)
	var req struct {
		Params struct {
			BlockNumber json.Number `json:"block_number"`
		} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(*sent, &req))
	assert.Equal(t, big, req.Params.BlockNumber.String())
}

// TestWithResponseKeepsFloat64 pins the documented WithResponse behaviour:
// the caller's target gets the response as encoding/json decodes it by
// default, numbers as float64, so existing callers that read it keep
// working.
func TestWithResponseKeepsFloat64(t *testing.T) {
	client, _ := bigServer(t, `{"jsonrpc":"2.0","id":1,"result":{"error_level":0,"result":7,"code":null,"message":null,"data":null}}`)
	var target interface{}
	_, err := client.Call(context.Background(), "test", types.Opt(func(r *types.RequestOptions) { r.Target = &target }))
	require.NoError(t, err)
	result := target.(map[string]interface{})["result"].(map[string]interface{})
	assert.Equal(t, float64(7), result["result"])
	assert.Equal(t, float64(0), result["error_level"])
}
