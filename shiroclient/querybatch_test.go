package shiroclient_test

import (
	"context"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/svc/txctx"
)

const queryBatchOK = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":false,
   "result":[{"id":"w","error_level":0,"result":"written","code":null,"message":null,"data":null},
             {"id":"d","error_level":0,"result":{"name":"alice"},"code":null,"message":null,"data":null}]},
 "$commit_tx_id":"","$com_block_num":"0","$sim_block_num":"9"}`

const queryBatchFailed = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":2,"committed":false,"code":-32000,
   "message":"batch failed: request 0 failed: nope","data":{"failed_index":0},
   "result":[{"id":"a","error_level":2,"result":null,"code":-32000,"message":"nope","data":"nope"},
             {"id":"b","error_level":2,"result":null,"code":-32001,"message":"Batch aborted","data":{"batch_aborted":true,"failed_id":"a"}}]}}`

const queryBatchCommitted = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,"committed":true,
   "result":[{"id":0,"error_level":0,"result":"1","code":null,"message":null,"data":null}]},
 "$commit_tx_id":"tx-x","$com_block_num":"3","$sim_block_num":"2"}`

const queryBatchNoCommittedField = `{"jsonrpc":"2.0","id":"batch-1",
 "result":{"error_level":0,"code":null,"message":null,"data":null,
   "result":[{"id":0,"error_level":0,"result":"1","code":null,"message":null,"data":null}]},
 "$sim_block_num":"4"}`

func TestQueryBatch(t *testing.T) {
	client, got, _ := batchGateway(t, queryBatchOK)
	ctx := txctx.Context(context.Background())
	var received int
	resp, err := shiroclient.QueryBatch(ctx, client, []shiroclient.CallBatchRequest{
		{Method: "put", ID: "w", Params: []interface{}{"k", "v"}},
		{Method: "private_decode", ID: "d", Configs: []shiroclient.Config{
			shiroclient.WithTransientData("mxf", []byte("req")),
		}},
	},
		shiroclient.WithTransientData("csprng_seed_private", []byte("seed")),
		shiroclient.WithResponseReceiver(func(shiroclient.ShiroResponse) { received++ }))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.False(t, resp.Committed, "a QueryBatch never commits")
	assert.Empty(t, resp.TxID)
	assert.Equal(t, uint64(0), resp.CommitBlockNum)
	assert.Equal(t, uint64(9), resp.MaxSimBlockNum)
	assert.Equal(t, -1, resp.FailedIndex())
	assert.Equal(t, []interface{}{"w", "d"}, resp.IDs)
	require.Len(t, resp.Responses, 2)
	for i, r := range resp.Responses {
		require.Nil(t, r.Error(), "request %d", i)
		assert.Empty(t, r.TransactionID(), "request %d", i)
		assert.Equal(t, uint64(9), r.MaxSimBlockNum(), "request %d", i)
	}
	assert.JSONEq(t, `{"name":"alice"}`, string(resp.Responses[1].ResultJSON()))
	assert.Equal(t, 2, received, "the receiver sees every response")
	assert.Empty(t, txctx.GetTransactionDetails(ctx).TransactionID, "nothing was committed")

	req := <-got
	assert.Equal(t, "QueryBatch", req["method"])
	params := req["params"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"csprng_seed_private": hex.EncodeToString([]byte("seed"))}, params["transient"])
	assert.Equal(t, []interface{}{
		map[string]interface{}{"method": "put", "params": []interface{}{"k", "v"}, "id": "w"},
		map[string]interface{}{"method": "private_decode", "params": []interface{}{}, "id": "d",
			"transient": map[string]interface{}{"mxf": hex.EncodeToString([]byte("req"))}},
	}, params["requests"])
}

func TestQueryBatchFailureIsBatchError(t *testing.T) {
	client, _, _ := batchGateway(t, queryBatchFailed)
	resp, err := shiroclient.QueryBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", ID: "a"},
		{Method: "b", ID: "b"},
	})
	var batchErr *shiroclient.CallBatchError
	require.True(t, errors.As(err, &batchErr), "got %T: %v", err, err)
	assert.Equal(t, 0, batchErr.Index)
	assert.Equal(t, "a", batchErr.ID)
	require.NotNil(t, resp, "the responses come with the error")
	assert.False(t, resp.Committed)
	failedID, aborted := shiroclient.CallBatchAborted(resp.Responses[1].Error())
	assert.True(t, aborted, "later requests are aborted, not run")
	assert.Equal(t, "a", failedID)
}

func TestQueryBatchCommittedIsContradiction(t *testing.T) {
	client, _, _ := batchGateway(t, queryBatchCommitted)
	resp, err := shiroclient.QueryBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "a"}})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "never commits")
}

func TestQueryBatchCommittedFieldOptional(t *testing.T) {
	client, _, _ := batchGateway(t, queryBatchNoCommittedField)
	resp, err := shiroclient.QueryBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "a"}})
	require.NoError(t, err)
	assert.False(t, resp.Committed)
	assert.Equal(t, uint64(4), resp.MaxSimBlockNum)
}

func TestQueryBatchOldGateway(t *testing.T) {
	client, _, _ := batchGateway(t, oldGateway)
	_, err := shiroclient.QueryBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "a"}})
	require.True(t, errors.Is(err, shiroclient.ErrQueryBatchNotSupported), "got %v", err)
	assert.False(t, errors.Is(err, shiroclient.ErrCallBatchNotSupported))
	assert.Contains(t, err.Error(), "QueryBatch")
}

func TestQueryBatchClientWithoutSupport(t *testing.T) {
	_, err := shiroclient.QueryBatch(context.Background(), plainClient{},
		[]shiroclient.CallBatchRequest{{Method: "a"}})
	require.True(t, errors.Is(err, shiroclient.ErrQueryBatchNotSupported), "got %v", err)
}

func TestQueryBatchMockNotSupported(t *testing.T) {
	client, err := shiroclient.NewMock(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	_, ok := client.(shiroclient.QueryBatcher)
	require.True(t, ok, "the mock client implements QueryBatcher")
	_, err = shiroclient.QueryBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "healthcheck"}})
	require.True(t, errors.Is(err, shiroclient.ErrQueryBatchNotSupported), "got %v", err)
}

func TestQueryBatchSameConfigRules(t *testing.T) {
	client, _, hits := batchGateway(t, queryBatchOK)
	_, err := shiroclient.QueryBatch(context.Background(), client,
		[]shiroclient.CallBatchRequest{{Method: "a"}},
		shiroclient.WithTransientData("mxf", []byte("x")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ShiroClient.QueryBatch")
	assert.Contains(t, err.Error(), "CallBatchRequest's Configs")

	_, err = shiroclient.QueryBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", Configs: []shiroclient.Config{shiroclient.WithMSPFilter([]string{"Org1MSP"})}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WithMSPFilter")

	_, err = shiroclient.QueryBatch(context.Background(), client, nil)
	require.Error(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(hits), "nothing is sent")
}
