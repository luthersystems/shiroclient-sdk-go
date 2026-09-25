package mock

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
	"github.com/luthersystems/svc/txctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batchFake is a plugin.Substrate that runs batches.  Only the batch methods
// are used; the embedded nil Substrate panics on anything else.
type batchFake struct {
	plugin.Substrate
	resp     *plugin.BatchResponse
	err      error
	gotTag   string
	gotReqs  []plugin.BatchRequestArgs
	gotOpts  *plugin.ConcreteRequestOptions
	gotQuery bool
}

func (f *batchFake) CallBatch(tag string, reqs []plugin.BatchRequestArgs, opts *plugin.ConcreteRequestOptions) (*plugin.BatchResponse, error) {
	f.gotTag, f.gotReqs, f.gotOpts = tag, reqs, opts
	return f.resp, f.err
}

func (f *batchFake) QueryBatch(tag string, reqs []plugin.BatchRequestArgs, opts *plugin.ConcreteRequestOptions) (*plugin.BatchResponse, error) {
	f.gotQuery = true
	return f.CallBatch(tag, reqs, opts)
}

// noBatch is a plugin.Substrate without batch methods.
type noBatch struct{ plugin.Substrate }

func okBatch(txID string, committed bool, n int) *plugin.BatchResponse {
	resp := &plugin.BatchResponse{TransactionID: txID, Committed: committed, FailedIndex: -1}
	for i := 0; i < n; i++ {
		resp.Responses = append(resp.Responses, &plugin.Response{ResultJSON: []byte(fmt.Sprintf(`"r%d"`, i)), TransactionID: txID})
		resp.IDs = append(resp.IDs, []byte(fmt.Sprint(i)))
	}
	return resp
}

func TestMockCallBatchThroughPlugin(t *testing.T) {
	fake := &batchFake{resp: okBatch("tx-1", true, 2)}
	fake.resp.IDs[0] = []byte(`"a"`)
	c := &mockShiroClient{substrate: fake, tag: "tag-1"}
	ctx := txctx.Context(context.Background())

	resp, err := c.CallBatch(ctx, []types.CallBatchRequest{
		{Method: "put", Params: []interface{}{"k", "v"}, ID: "a",
			Configs: []types.Config{types.Opt(func(r *types.RequestOptions) { r.Transient["secret"] = []byte("alice") })}},
		{Method: "get"},
	}, types.CSPRNGSeedConfig([]byte("seed")))
	require.NoError(t, err)

	assert.Equal(t, "tag-1", fake.gotTag)
	assert.False(t, fake.gotQuery)
	require.Len(t, fake.gotReqs, 2)
	assert.Equal(t, plugin.BatchRequestArgs{
		Method: "put", Params: []byte(`["k","v"]`), ID: []byte(`"a"`),
		Transient: map[string][]byte{"secret": []byte("alice")},
	}, fake.gotReqs[0], "a request's own transient data goes with it alone")
	assert.Equal(t, plugin.BatchRequestArgs{Method: "get", Params: []byte(`[]`)}, fake.gotReqs[1])
	assert.Equal(t, []byte("seed"), fake.gotOpts.Transient["csprng_seed_private"])
	assert.NotContains(t, fake.gotOpts.Transient, "secret", "request data is not shared")
	assert.NotEmpty(t, fake.gotOpts.Timestamp)

	assert.True(t, resp.Committed)
	assert.Equal(t, "tx-1", resp.TxID)
	assert.Equal(t, []interface{}{"a", float64(1)}, resp.IDs)
	require.Len(t, resp.Responses, 2)
	assert.JSONEq(t, `"r1"`, string(resp.Responses[1].ResultJSON()))
	assert.Equal(t, "tx-1", resp.Responses[1].TransactionID())
	assert.Equal(t, "tx-1", txctx.GetTransactionDetails(ctx).TransactionID)
}

func TestMockCallBatchFailure(t *testing.T) {
	fake := &batchFake{resp: &plugin.BatchResponse{
		Responses: []*plugin.Response{
			{HasError: true, ErrorCode: types.CodeBatchAborted, ErrorMessage: "Batch aborted", ErrorJSON: []byte(`{"batch_aborted":true,"failed_id":"d"}`)},
			{HasError: true, ErrorCode: types.CodeForcedNoCommit, ErrorMessage: "forced no commit", ErrorJSON: []byte(`{"failed_id":"d"}`)},
		},
		IDs:         [][]byte{[]byte(`"w"`), []byte(`"d"`)},
		FailedIndex: 1,
	}}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	resp, err := c.CallBatch(context.Background(), []types.CallBatchRequest{
		{Method: "put", ID: "w"}, {Method: "private_decode", ID: "d"},
	})
	var batchErr *types.CallBatchError
	require.True(t, errors.As(err, &batchErr), "got %T: %v", err, err)
	assert.Equal(t, 1, batchErr.Index)
	assert.Equal(t, "d", batchErr.ID)
	assert.Equal(t, types.CodeForcedNoCommit, batchErr.Err.Code())
	require.NotNil(t, resp)
	assert.False(t, resp.Committed)
	failedID, aborted := types.CallBatchAborted(resp.Responses[0].Error())
	assert.True(t, aborted)
	assert.Equal(t, "d", failedID)
}

func TestMockQueryBatchThroughPlugin(t *testing.T) {
	fake := &batchFake{resp: okBatch("", false, 1)}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	resp, err := c.QueryBatch(context.Background(), []types.CallBatchRequest{{Method: "private_decode"}})
	require.NoError(t, err)
	assert.True(t, fake.gotQuery)
	assert.False(t, resp.Committed)
	assert.Empty(t, resp.TxID)

	fake.resp = okBatch("tx", true, 1)
	_, err = c.QueryBatch(context.Background(), []types.CallBatchRequest{{Method: "a"}})
	require.Error(t, err, "a committed QueryBatch is a contradiction")
}

func TestMockBatchNotSupported(t *testing.T) {
	for name, sub := range map[string]plugin.Substrate{
		"no BatchSubstrate":    noBatch{},
		"ErrBatchNotSupported": &batchFake{err: fmt.Errorf("wrapped: %w", plugin.ErrBatchNotSupported)},
	} {
		t.Run(name, func(t *testing.T) {
			c := &mockShiroClient{substrate: sub, tag: "t"}
			_, err := c.CallBatch(context.Background(), []types.CallBatchRequest{{Method: "a"}})
			assert.True(t, errors.Is(err, types.ErrCallBatchNotSupported), "got %v", err)
			_, err = c.QueryBatch(context.Background(), []types.CallBatchRequest{{Method: "a"}})
			assert.True(t, errors.Is(err, types.ErrQueryBatchNotSupported), "got %v", err)
		})
	}
}

func TestMockBatchValidatesBeforeSending(t *testing.T) {
	fake := &batchFake{resp: okBatch("", false, 1)}
	c := &mockShiroClient{substrate: fake, tag: "t"}
	_, err := c.CallBatch(context.Background(), []types.CallBatchRequest{{Method: "a"}},
		types.Opt(func(r *types.RequestOptions) { r.Transient["mxf"] = []byte("x") }))
	require.Error(t, err)
	_, err = c.CallBatch(context.Background(), []types.CallBatchRequest{
		{Method: "a", Configs: []types.Config{types.Opt(func(r *types.RequestOptions) { r.MinEndorsers = 2 })}},
	})
	require.Error(t, err)
	assert.Nil(t, fake.gotReqs, "nothing reaches the plugin")
}

// TestMockBatchReleasedPlugin pins that the released substratehcp, which has
// no batch methods, still yields the not-supported errors, with a private
// and a shared plugin process alike.
func TestMockBatchReleasedPlugin(t *testing.T) {
	requirePlugin(t)
	t.Cleanup(func() { _ = ShutdownSharedPlugins() })
	for name, opts := range map[string][]mock.Option{
		"private": nil,
		"shared":  {mock.WithSharedPlugin()},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestMock(t, opts...)
			t.Cleanup(func() { _ = c.Close() })
			_, err := c.CallBatch(context.Background(), []types.CallBatchRequest{{Method: "healthcheck"}})
			assert.True(t, errors.Is(err, types.ErrCallBatchNotSupported), "got %v", err)
			_, err = c.QueryBatch(context.Background(), []types.CallBatchRequest{{Method: "healthcheck"}})
			assert.True(t, errors.Is(err, types.ErrQueryBatchNotSupported), "got %v", err)
		})
	}
}
