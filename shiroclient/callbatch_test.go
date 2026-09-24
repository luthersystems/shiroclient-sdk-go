package shiroclient_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/svc/txctx"
)

// batchGateway starts a gateway stub that answers every JSON-RPC request with
// reply and records each request body it receives.  The replies are the
// shapes the shiroclient gateway of luthersystems/substrate#521 produces
// (cmd/shiroclient/cmd/gateway_batch_test.go there).
func batchGateway(t *testing.T, reply string) (shiroclient.ShiroClient, <-chan map[string]interface{}, *int32) {
	t.Helper()
	got := make(chan map[string]interface{}, 8)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	client := shiroclient.NewRPC([]shiroclient.Config{
		shiroclient.WithEndpoint(srv.URL),
		shiroclient.WithHTTPClient(srv.Client()),
	})
	return client, got, &hits
}

const batchCommitted = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":true,
   "result":[{"id":"r1","error_level":0,"result":{"key":"a"},"code":null,"message":null,"data":null},
             {"id":1,"error_level":0,"result":"1","code":null,"message":null,"data":null}]},
 "$commit_tx_id":"tx-batch","$com_block_num":"12","$sim_block_num":"11"}`

const batchSpoiled = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":2,"committed":false,"code":-32000,
   "message":"batch not committed: request 1 failed: nope","data":{"failed_index":1},
   "result":[{"id":"p","error_level":2,"result":null,"code":-32001,"message":"Batch aborted","data":{"batch_aborted":true,"failed_id":"f"}},
             {"id":"f","error_level":2,"result":null,"code":-32000,"message":"nope","data":"nope"},
             {"id":"q","error_level":2,"result":null,"code":-32001,"message":"Batch aborted","data":{"batch_aborted":true,"failed_id":"f"}}]}}`

const batchReadOnly = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":false,
   "result":[{"id":0,"error_level":0,"result":"1","code":null,"message":null,"data":null}]},
 "$commit_tx_id":"","$com_block_num":"0","$sim_block_num":"7"}`

const batchOutcomeUnknown = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":1,"result":null,"code":1,
   "message":"transaction outcome unknown: txid=tx-amb: transaction timeout",
   "data":{"outcome":"unknown","tx_id":"tx-amb"}}}`

const oldGateway = `{"jsonrpc":"2.0","id":"batch-1","error":{"code":-32601,"message":"method not found"}}`

func TestCallBatchCommitted(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)

	resp, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "put", Params: []interface{}{"a", "1"}, ID: "r1"},
		{Method: "get"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Committed)
	assert.Equal(t, "tx-batch", resp.TxID)
	assert.Equal(t, uint64(12), resp.CommitBlockNum)
	assert.Equal(t, uint64(11), resp.MaxSimBlockNum)
	assert.Equal(t, -1, resp.FailedIndex())
	assert.Equal(t, []interface{}{"r1", float64(1)}, resp.IDs)
	require.Len(t, resp.Responses, 2)
	for i, r := range resp.Responses {
		require.Nil(t, r.Error(), "request %d", i)
		assert.Equal(t, "tx-batch", r.TransactionID(), "every request shares the batch's one transaction")
	}
	var first map[string]string
	require.NoError(t, resp.Responses[0].UnmarshalTo(&first))
	assert.Equal(t, map[string]string{"key": "a"}, first)
	assert.JSONEq(t, `"1"`, string(resp.Responses[1].ResultJSON()))

	req := <-got
	assert.Equal(t, "2.0", req["jsonrpc"])
	assert.Equal(t, "CallBatch", req["method"])
	params := req["params"].(map[string]interface{})
	assert.Equal(t, []interface{}{
		map[string]interface{}{"method": "put", "params": []interface{}{"a", "1"}, "id": "r1"},
		// A missing id is left to the server (it uses the index); missing
		// params are sent as an empty array, which the gateway requires.
		map[string]interface{}{"method": "get", "params": []interface{}{}},
	}, params["requests"])
	assert.Equal(t, map[string]interface{}{}, params["transient"])
}

func TestCallBatchResponseReceiver(t *testing.T) {
	client, _, _ := batchGateway(t, batchCommitted)
	var received []shiroclient.ShiroResponse
	_, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "put"}, {Method: "get"}},
		shiroclient.WithResponseReceiver(func(r shiroclient.ShiroResponse) { received = append(received, r) }))
	require.NoError(t, err)
	require.Len(t, received, 2, "the receiver sees every request's response")
}

func TestCallBatchFailureCommitsNothing(t *testing.T) {
	client, _, _ := batchGateway(t, batchSpoiled)

	resp, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "put", ID: "p"},
		{Method: "failwrite", ID: "f"},
		{Method: "put", ID: "q"},
	})
	require.Error(t, err)

	var batchErr *shiroclient.CallBatchError
	require.True(t, errors.As(err, &batchErr), "got %T: %v", err, err)
	assert.Equal(t, 1, batchErr.Index)
	assert.Equal(t, "f", batchErr.ID)
	require.NotNil(t, batchErr.Err)
	assert.Equal(t, -32000, batchErr.Err.Code())
	assert.Equal(t, "nope", batchErr.Err.Message())
	assert.JSONEq(t, `"nope"`, string(batchErr.Err.DataJSON()))
	assert.Contains(t, err.Error(), "batch not committed")
	assert.Contains(t, err.Error(), "request 1")
	assert.True(t, errors.As(fmt.Errorf("wrapped: %w", err), &batchErr))
	assert.False(t, errors.Is(err, shiroclient.ErrOutcomeUnknown))
	assert.False(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported))

	// The per-request results are still returned, so a caller can see why.
	require.NotNil(t, resp)
	assert.False(t, resp.Committed)
	assert.Empty(t, resp.TxID)
	assert.Equal(t, 1, resp.FailedIndex())
	require.Len(t, resp.Responses, 3)
	for i, r := range resp.Responses {
		require.NotNil(t, r.Error(), "request %d: no request of a spoiled batch succeeds", i)
		assert.Empty(t, r.TransactionID(), "request %d", i)
		failedID, aborted := shiroclient.CallBatchAborted(r.Error())
		assert.Equal(t, i != 1, aborted, "request %d", i)
		if aborted {
			assert.Equal(t, "f", failedID, "request %d names the request that failed", i)
		}
	}
}

func TestCallBatchReadOnlyIsNotCommitted(t *testing.T) {
	client, _, _ := batchGateway(t, batchReadOnly)
	resp, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "get", Params: []interface{}{"a"}}})
	require.NoError(t, err)
	assert.False(t, resp.Committed)
	assert.Empty(t, resp.TxID)
	assert.Equal(t, -1, resp.FailedIndex())
	assert.JSONEq(t, `"1"`, string(resp.Responses[0].ResultJSON()))
}

func TestCallBatchOutcomeUnknown(t *testing.T) {
	client, _, _ := batchGateway(t, batchOutcomeUnknown)
	resp, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "put"}, {Method: "put"}})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, shiroclient.ErrOutcomeUnknown))
	assert.True(t, shiroclient.IsTimeoutError(err))
	txID, ok := shiroclient.OutcomeUnknownTxID(err)
	assert.True(t, ok)
	assert.Equal(t, "tx-amb", txID)
	var batchErr *shiroclient.CallBatchError
	assert.False(t, errors.As(err, &batchErr), "an unknown outcome is not a known failure")
}

func TestCallBatchOldGateway(t *testing.T) {
	client, _, hits := batchGateway(t, oldGateway)
	resp, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "put"}})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported), "got %v", err)
	assert.Equal(t, int32(1), atomic.LoadInt32(hits), "the batch is not retried as single calls")
}

func TestCallBatchOtherJSONRPCError(t *testing.T) {
	client, _, _ := batchGateway(t, `{"jsonrpc":"2.0","id":"batch-1","error":{"code":-32602,"message":"invalid params"}}`)
	_, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "put"}})
	require.Error(t, err)
	assert.False(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported))
	assert.Contains(t, err.Error(), "invalid params")
}

func TestCallBatchForwardsOptions(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{
			{Method: "put", Params: map[string]interface{}{"k": "v"}},
			{Method: "get", ID: 7},
		},
		shiroclient.WithoutTargetEndpoints([]string{"peer-restoring"}),
		shiroclient.WithTargetEndpoints([]string{"peer0"}),
		shiroclient.WithTransientData("tkey", []byte("secret")),
		shiroclient.WithMSPFilter([]string{"Org1MSP"}),
		shiroclient.WithMinEndorsers(2),
		shiroclient.WithCreator("Org2MSP"),
		shiroclient.WithDependentTxID("tx-dep"),
		shiroclient.WithDependentBlock("5"),
		shiroclient.WithPhylumVersion("v1.2.3"),
		shiroclient.WithDisableWritePolling(true),
		shiroclient.WithTimestampGenerator(func(context.Context) string { return "2026-01-02T03:04:05Z" }),
	)
	require.NoError(t, err)

	params := (<-got)["params"].(map[string]interface{})
	assert.Equal(t, []interface{}{"peer-restoring"}, params["not_target_endpoints"])
	assert.Equal(t, []interface{}{"peer0"}, params["target_endpoints"])
	assert.Equal(t, []interface{}{"Org1MSP"}, params["msp_filter"])
	assert.EqualValues(t, 2, params["min_endorsers"])
	assert.Equal(t, "Org2MSP", params["creator_msp_id"])
	assert.Equal(t, "tx-dep", params["dependent_txid"])
	assert.Equal(t, "5", params["dependent_block"])
	assert.Equal(t, "v1.2.3", params["phylum_version"])
	assert.Equal(t, true, params["disable_write_polling"])
	assert.Equal(t, map[string]interface{}{
		"tkey":               hex.EncodeToString([]byte("secret")),
		"timestamp_override": hex.EncodeToString([]byte("2026-01-02T03:04:05Z")),
	}, params["transient"], "transient data is sent once and shared by every request")
	assert.Equal(t, []interface{}{
		map[string]interface{}{"method": "put", "params": map[string]interface{}{"k": "v"}},
		map[string]interface{}{"method": "get", "params": []interface{}{}, "id": float64(7)},
	}, params["requests"])
	assert.NotContains(t, params, "method", "a batch has no top-level phylum method")
}

func TestCallBatchRejectsInvalidRequests(t *testing.T) {
	client, _, hits := batchGateway(t, batchCommitted)
	for name, reqs := range map[string][]shiroclient.CallBatchRequest{
		"empty":          nil,
		"missing method": {{Method: "put"}, {Params: []interface{}{}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := shiroclient.CallBatch(context.Background(), client, reqs)
			require.Error(t, err)
		})
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(hits), "an invalid batch is never sent")
}

// plainClient implements ShiroClient but not CallBatcher, as a third-party
// implementation of the interface does.
type plainClient struct{ shiroclient.ShiroClient }

func TestCallBatchClientWithoutSupport(t *testing.T) {
	_, err := shiroclient.CallBatch(context.Background(), plainClient{},
		[]shiroclient.CallBatchRequest{{Method: "put"}})
	require.True(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported), "got %v", err)
}

func TestCallBatchMockNotSupported(t *testing.T) {
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	initClient(t, client, testPhylum)

	_, ok := client.(shiroclient.CallBatcher)
	require.True(t, ok, "the mock client implements CallBatcher")
	_, err = shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "healthcheck"}})
	require.True(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported), "got %v", err)
}

// batchTimeoutNoOutcome is how the #521 gateway, before it is stacked on
// luthersystems/substrate#515, answers a batch whose commit timed out: the
// timeout code, and no outcome data.  The batch may still have committed.
const batchTimeoutNoOutcome = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":1,"result":null,"code":1,"message":"transaction timeout","data":null}}`

func TestCallBatchTimeoutWithoutOutcomeData(t *testing.T) {
	client, _, _ := batchGateway(t, batchTimeoutNoOutcome)
	resp, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "put"}, {Method: "put"}})
	require.Error(t, err)
	assert.True(t, shiroclient.IsTimeoutError(err), "a timed-out batch may have committed")
	// Never presented as a known, retry-safe failure.
	assert.Nil(t, resp, "no response claims the batch was not committed")
	var batchErr *shiroclient.CallBatchError
	assert.False(t, errors.As(err, &batchErr))
	assert.False(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported))
	_, ok := shiroclient.OutcomeUnknownTxID(err)
	assert.False(t, ok, "without #515 the gateway sends no tx id")
}

func TestCallBatchNilParamsSentAsEmptyArray(t *testing.T) {
	type obj struct{ K string }
	client, got, _ := batchGateway(t, `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":false,
   "result":[{"id":0,"error_level":0,"result":null,"code":null,"message":null,"data":null},
             {"id":1,"error_level":0,"result":null,"code":null,"message":null,"data":null},
             {"id":2,"error_level":0,"result":null,"code":null,"message":null,"data":null},
             {"id":3,"error_level":0,"result":null,"code":null,"message":null,"data":null}]}}`)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", Params: []string(nil)},
		{Method: "b", Params: map[string]int(nil)},
		{Method: "c", Params: (*obj)(nil)},
		{Method: "d", Params: &obj{K: "v"}},
	})
	require.NoError(t, err)
	reqs := (<-got)["params"].(map[string]interface{})["requests"].([]interface{})
	for i, want := range []interface{}{
		[]interface{}{}, []interface{}{}, []interface{}{}, map[string]interface{}{"K": "v"},
	} {
		assert.Equal(t, want, reqs[i].(map[string]interface{})["params"], "request %d", i)
	}
}

func TestCallBatchRejectsScalarParams(t *testing.T) {
	client, _, hits := batchGateway(t, batchCommitted)
	for name, params := range map[string]interface{}{
		"string": "a",
		"number": 7,
		"bool":   true,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := shiroclient.CallBatch(context.Background(), client,
				[]shiroclient.CallBatchRequest{{Method: "put", Params: params}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "array or an object")
		})
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(hits))
}

func TestCallBatchRequestIDs(t *testing.T) {
	client, got, hits := batchGateway(t, batchCommitted)
	for name, id := range map[string]interface{}{
		"object": map[string]interface{}{"a": 1},
		"array":  []interface{}{1},
		"bool":   true,
		"struct": struct{}{},
		// Above 2^53 the gateway's float64 decoding would change the id.
		"int above 2^53":     int64(1)<<53 + 1,
		"uint above 2^53":    uint64(1) << 60,
		"json.Number > 2^53": json.Number("9007199254740993"),
		"bad json.Number":    json.Number("abc"),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := shiroclient.CallBatch(context.Background(), client,
				[]shiroclient.CallBatchRequest{{Method: "put"}, {Method: "get", ID: id}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "request 1")
			assert.Contains(t, err.Error(), "id")
		})
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(hits), "an invalid id is never sent")

	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", ID: "s"}, {Method: "b", ID: 3},
	})
	require.NoError(t, err)
	<-got
	type orderID string
	type seq int32
	for _, id := range []interface{}{int64(1), uint8(2), float64(1.5), json.Number("4"),
		int64(1) << 53, orderID("o-1"), seq(7)} {
		_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
			{Method: "a", ID: id}, {Method: "b"},
		})
		require.NoError(t, err, "id %#v", id)
		<-got
	}
}

func TestCallBatchContradictoryResponse(t *testing.T) {
	// error_level 0 and committed, yet a request failed: the gateway
	// contradicts itself, so neither success nor a clean failure is claimed.
	client, _, _ := batchGateway(t, `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":true,
   "result":[{"id":0,"error_level":0,"result":"ok","code":null,"message":null,"data":null},
             {"id":1,"error_level":2,"result":null,"code":-32000,"message":"nope","data":null}]},
 "$commit_tx_id":"tx-odd","$com_block_num":"3","$sim_block_num":"2"}`)
	ctx := txctx.Context(context.Background())
	var received int
	resp, err := shiroclient.CallBatch(ctx, client,
		[]shiroclient.CallBatchRequest{{Method: "a"}, {Method: "b"}},
		shiroclient.WithResponseReceiver(func(shiroclient.ShiroResponse) { received++ }))
	require.Error(t, err)
	assert.Nil(t, resp)
	var batchErr *shiroclient.CallBatchError
	assert.False(t, errors.As(err, &batchErr), "not reported as a known uncommitted failure")
	assert.Empty(t, txctx.GetTransactionDetails(ctx).TransactionID, "txctx is stamped only after every check passes")
	assert.Zero(t, received)
}

func TestCallBatchStampsTransactionDetails(t *testing.T) {
	client, _, _ := batchGateway(t, batchCommitted)
	ctx := txctx.Context(context.Background())
	_, err := shiroclient.CallBatch(ctx, client,
		[]shiroclient.CallBatchRequest{{Method: "a"}, {Method: "b"}})
	require.NoError(t, err)
	assert.Equal(t, "tx-batch", txctx.GetTransactionDetails(ctx).TransactionID)
}

// A phylum's own failure can carry data that looks like the abort marker.
// Only the reserved code -32001, which substrate's router never lets a phylum
// produce, makes an element an aborted one.
func TestCallBatchAbortedRequiresReservedCode(t *testing.T) {
	client, _, _ := batchGateway(t, `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":2,"code":-32000,"message":"Server error","committed":false,
   "data":{"failed_index":0},
   "result":[{"id":"f","error_level":2,"result":null,"code":-32000,"message":"Server error","data":{"batch_aborted":true,"failed_id":"x"}},
             {"id":"q","error_level":2,"result":null,"code":-32001,"message":"Batch aborted","data":{"batch_aborted":true,"failed_id":"f"}}]}}`)
	resp, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", ID: "f"}, {Method: "b", ID: "q"},
	})
	require.Error(t, err)
	require.NotNil(t, resp)
	_, spoofed := shiroclient.CallBatchAborted(resp.Responses[0].Error())
	assert.False(t, spoofed, "a phylum error with abort-shaped data is not an abort")
	failedID, aborted := shiroclient.CallBatchAborted(resp.Responses[1].Error())
	assert.True(t, aborted)
	assert.Equal(t, "f", failedID)
	assert.Equal(t, 0, resp.FailedIndex())
}

func TestCallBatchPerRequestTransient(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "deposit", Transient: map[string][]byte{"secret": []byte("alice")}},
		{Method: "deposit", Transient: map[string][]byte{"secret": []byte("bob")}},
	})
	require.NoError(t, err)
	params := (<-got)["params"].(map[string]interface{})
	reqs := params["requests"].([]interface{})
	for i, want := range []string{"alice", "bob"} {
		assert.Equal(t, map[string]interface{}{"secret": hex.EncodeToString([]byte(want))},
			reqs[i].(map[string]interface{})["transient"], "request %d has its own value for the same key", i)
	}
	assert.Equal(t, map[string]interface{}{}, params["transient"], "nothing is shared")
}

func TestCallBatchSharedAndPerRequestTransient(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", Transient: map[string][]byte{"secret": []byte("own")}},
		{Method: "b", Transient: map[string][]byte{}},
	}, shiroclient.WithTransientData("shared", []byte("both")))
	require.NoError(t, err)
	params := (<-got)["params"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"shared": hex.EncodeToString([]byte("both"))}, params["transient"])
	reqs := params["requests"].([]interface{})
	assert.Equal(t, map[string]interface{}{"secret": hex.EncodeToString([]byte("own"))},
		reqs[0].(map[string]interface{})["transient"])
	assert.NotContains(t, reqs[1].(map[string]interface{}), "transient", "an empty map sends no transient field")
}

func TestCallBatchTransientOmittedWhenNil(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "a"}, {Method: "b"}})
	require.NoError(t, err)
	for i, r := range (<-got)["params"].(map[string]interface{})["requests"].([]interface{}) {
		assert.NotContains(t, r.(map[string]interface{}), "transient", "request %d", i)
	}
}

func TestCallBatchRejectsReservedTransientKeys(t *testing.T) {
	client, _, hits := batchGateway(t, batchCommitted)
	cases := map[string]func() error{
		"per-request $batch/ key": func() error {
			_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
				{Method: "a"},
				{Method: "b", Transient: map[string][]byte{"$batch/0/secret": []byte("x")}},
			})
			return err
		},
		"shared $batch/ key": func() error {
			_, err := shiroclient.CallBatch(context.Background(), client,
				[]shiroclient.CallBatchRequest{{Method: "a"}, {Method: "b"}},
				shiroclient.WithTransientData("$batch/1/secret", []byte("x")))
			return err
		},
		"Call $batch/ key": func() error {
			_, err := client.Call(context.Background(), "a",
				shiroclient.WithTransientDataMap(map[string][]byte{"$batch/0/k": []byte("x")}))
			return err
		},
		"empty per-request key": func() error {
			_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
				{Method: "a", Transient: map[string][]byte{"": []byte("x")}},
				{Method: "b"},
			})
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "transient")
		})
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(hits), "nothing is sent")
}
