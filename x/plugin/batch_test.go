package plugin

import (
	"errors"
	"net"
	"net/rpc"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSubstrate is a Substrate that records the batches it is given.  Only
// the batch methods are used; the embedded nil Substrate panics on anything
// else.
type fakeSubstrate struct {
	Substrate
	resp     *BatchResponse
	err      error
	gotTag   string
	gotReqs  []BatchRequestArgs
	gotOpts  *ConcreteRequestOptions
	gotQuery bool
}

func (f *fakeSubstrate) CallBatch(tag string, reqs []BatchRequestArgs, opts *ConcreteRequestOptions) (*BatchResponse, error) {
	f.gotTag, f.gotReqs, f.gotOpts = tag, reqs, opts
	return f.resp, f.err
}

func (f *fakeSubstrate) QueryBatch(tag string, reqs []BatchRequestArgs, opts *ConcreteRequestOptions) (*BatchResponse, error) {
	f.gotQuery = true
	return f.CallBatch(tag, reqs, opts)
}

// batchOnly implements Substrate but not BatchSubstrate: an older substrate
// built against this SDK version.
type batchOnly struct{ Substrate }

// oldPluginServer stands in for a plugin binary built before the batch
// methods existed: net/rpc has no Plugin.CallBatch to dispatch to.
type oldPluginServer struct{}

func (oldPluginServer) HealthCheck(args *ArgsHealthCheck, resp *RespHealthCheck) error {
	resp.Suc = args.Nat
	return nil
}

// pipeClient serves rcvr as "Plugin" over an in-process connection, as
// go-plugin's net/rpc transport does, and returns the client side.
func pipeClient(t *testing.T, rcvr interface{}) *PluginRPC {
	t.Helper()
	srv := rpc.NewServer()
	require.NoError(t, srv.RegisterName("Plugin", rcvr))
	c1, c2 := net.Pipe()
	go srv.ServeConn(c1)
	client := rpc.NewClient(c2)
	t.Cleanup(func() { _ = client.Close() })
	return &PluginRPC{client: client}
}

var _ BatchSubstrate = (*PluginRPC)(nil)

func TestPluginCallBatchRoundTrip(t *testing.T) {
	fake := &fakeSubstrate{resp: &BatchResponse{
		Responses: []*Response{
			{ResultJSON: []byte(`{"k":"v"}`), TransactionID: "tx-1"},
			{ResultJSON: []byte(`"2"`), TransactionID: "tx-1"},
		},
		IDs:           [][]byte{[]byte(`"a"`), []byte(`1`)},
		Committed:     true,
		TransactionID: "tx-1",
		FailedIndex:   -1,
	}}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})

	reqs := []BatchRequestArgs{
		{Method: "put", Params: []byte(`["a","1"]`), ID: []byte(`"a"`),
			Transient: map[string][]byte{"secret": []byte("alice")}},
		{Method: "get", Params: []byte(`[]`)},
	}
	opts := &ConcreteRequestOptions{
		Transient: map[string][]byte{"csprng_seed_private": []byte("seed")},
		Timestamp: "2026-01-02T03:04:05Z",
		Creator:   "Org1MSP",
	}
	resp, err := client.CallBatch("tag-1", reqs, opts)
	require.NoError(t, err)

	assert.Equal(t, "tag-1", fake.gotTag)
	assert.Equal(t, reqs, fake.gotReqs, "every request, with its own transient data, reaches the substrate")
	assert.Equal(t, opts, fake.gotOpts, "the batch's options carry the transaction-wide transient keys")
	assert.False(t, fake.gotQuery)
	assert.Equal(t, fake.resp, resp)
}

func TestPluginCallBatchFailure(t *testing.T) {
	fake := &fakeSubstrate{resp: &BatchResponse{
		Responses: []*Response{
			{HasError: true, ErrorCode: -32001, ErrorMessage: "Batch aborted", ErrorJSON: []byte(`{"batch_aborted":true,"failed_id":1}`)},
			{HasError: true, ErrorCode: -32002, ErrorMessage: "forced no commit", ErrorJSON: []byte(`{"failed_id":1}`)},
		},
		IDs:         [][]byte{[]byte(`0`), []byte(`1`)},
		FailedIndex: 1,
	}}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})
	resp, err := client.CallBatch("tag", []BatchRequestArgs{{Method: "put"}, {Method: "private_decode"}}, &ConcreteRequestOptions{})
	require.NoError(t, err, "a failed request is a result, not a transport error")
	assert.False(t, resp.Committed)
	assert.Empty(t, resp.TransactionID)
	assert.Equal(t, 1, resp.FailedIndex)
	assert.Equal(t, -32002, resp.Responses[1].ErrorCode)
	assert.Equal(t, -32001, resp.Responses[0].ErrorCode)
}

func TestPluginQueryBatchRoundTrip(t *testing.T) {
	fake := &fakeSubstrate{resp: &BatchResponse{
		Responses:   []*Response{{ResultJSON: []byte(`"decoded"`)}},
		IDs:         [][]byte{[]byte(`0`)},
		FailedIndex: -1,
	}}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})
	resp, err := client.QueryBatch("tag", []BatchRequestArgs{{Method: "private_decode", Transient: map[string][]byte{"mxf": []byte("x")}}}, &ConcreteRequestOptions{})
	require.NoError(t, err)
	assert.True(t, fake.gotQuery, "QueryBatch reaches the substrate's QueryBatch")
	assert.Equal(t, map[string][]byte{"mxf": []byte("x")}, fake.gotReqs[0].Transient)
	assert.False(t, resp.Committed, "a QueryBatch is never committed")
	assert.Empty(t, resp.TransactionID)
}

func TestPluginBatchError(t *testing.T) {
	fake := &fakeSubstrate{err: errors.New("mock tag not found")}
	client := pipeClient(t, &PluginRPCServer{Impl: fake})
	_, err := client.CallBatch("tag", []BatchRequestArgs{{Method: "a"}}, &ConcreteRequestOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock tag not found")
	assert.False(t, errors.Is(err, ErrBatchNotSupported))
}

func TestPluginBatchNotSupported(t *testing.T) {
	cases := map[string]interface{}{
		"impl without BatchSubstrate":       &PluginRPCServer{Impl: batchOnly{}},
		"impl reports ErrBatchNotSupported": &PluginRPCServer{Impl: &fakeSubstrate{err: ErrBatchNotSupported}},
		"plugin binary without the method":  oldPluginServer{},
	}
	for name, rcvr := range cases {
		t.Run(name, func(t *testing.T) {
			client := pipeClient(t, rcvr)
			_, err := client.CallBatch("tag", []BatchRequestArgs{{Method: "a"}}, &ConcreteRequestOptions{})
			require.True(t, errors.Is(err, ErrBatchNotSupported), "CallBatch: got %v", err)
			_, err = client.QueryBatch("tag", []BatchRequestArgs{{Method: "a"}}, &ConcreteRequestOptions{})
			require.True(t, errors.Is(err, ErrBatchNotSupported), "QueryBatch: got %v", err)
		})
	}
}
