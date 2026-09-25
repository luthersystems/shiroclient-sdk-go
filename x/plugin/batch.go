package plugin

import (
	"errors"
	"fmt"
	"net/rpc"
	"strings"
)

// ErrBatchNotSupported is returned by PluginRPC.CallBatch and QueryBatch
// when the plugin cannot run batches: its binary predates the batch methods,
// its Substrate does not implement BatchSubstrate, or the implementation
// itself returned an error matching ErrBatchNotSupported.  Nothing was run.
var ErrBatchNotSupported = errors.New("substrate plugin does not support batches")

// BatchSubstrate is implemented by a Substrate that can run batches.  It is
// separate from Substrate so that adding it did not break implementations of
// that interface: PluginRPCServer type-asserts its Impl to BatchSubstrate
// and answers "not supported" when the assertion fails.
//
// CallBatch runs the requests in order as ONE transaction, all or nothing,
// and commits it when every request succeeds and the batch wrote state.
// QueryBatch runs them the same way in one simulation and never commits.
// In both, the first failed request stops the batch: that request's
// Response carries its own error (-32002, CodeForcedNoCommit, for a request
// that forces no commit in a CallBatch), and every other request's carries
// -32001 (CodeBatchAborted) with data {"batch_aborted":true,"failed_id":...}.
//
// options are the batch's own options, as for Call; options.Transient holds
// only the transaction-wide keys (csprng_seed_private, timestamp_override,
// traceparent, tracestate), and options.Params is unused.  Each request's own
// transient data is in its BatchRequestArgs.Transient.  A failed request is a
// result (BatchResponse), not an error: the error return is for a batch that
// could not run at all.  Return an error matching ErrBatchNotSupported when
// the substrate cannot run batches.
type BatchSubstrate interface {
	CallBatch(tag string, requests []BatchRequestArgs, options *ConcreteRequestOptions) (*BatchResponse, error)
	QueryBatch(tag string, requests []BatchRequestArgs, options *ConcreteRequestOptions) (*BatchResponse, error)
}

// BatchRequestArgs is one request of a batch.
type BatchRequestArgs struct {
	// Method is the phylum endpoint to call.
	Method string
	// Params is the request's JSON-encoded parameters: an array or an
	// object.
	Params []byte
	// ID is the request's JSON-encoded JSON-RPC id (a string or a number),
	// or nil to use the request's index in the batch.
	ID []byte
	// Transient is transient data for this request only.  Its keys are
	// non-empty, never transaction-wide and never start with "$batch/".
	Transient map[string][]byte
}

// BatchResponse is the result of a batch.
type BatchResponse struct {
	// Responses holds one response per request, in request order.
	Responses []*Response
	// IDs holds each response's JSON-encoded JSON-RPC id.
	IDs [][]byte
	// TransactionID is the committed transaction's ID; empty when the batch
	// was not committed, and always for a QueryBatch.
	TransactionID string
	// FailedIndex is the index of the request that failed the batch, or -1.
	FailedIndex int
	// Committed reports whether the batch was committed: it wrote state and
	// no request failed.  Always false for a QueryBatch.
	Committed bool
}

// ArgsCallBatch encodes the arguments to CallBatch
type ArgsCallBatch struct {
	Tag      string
	Requests []BatchRequestArgs
	Options  *ConcreteRequestOptions
}

// RespCallBatch encodes the response from CallBatch
type RespCallBatch struct {
	Response *BatchResponse
	Err      *Error
	// NotSupported reports that the plugin cannot run batches.
	NotSupported bool
}

// ArgsQueryBatch encodes the arguments to QueryBatch
type ArgsQueryBatch struct {
	Tag      string
	Requests []BatchRequestArgs
	Options  *ConcreteRequestOptions
}

// RespQueryBatch encodes the response from QueryBatch
type RespQueryBatch struct {
	Response *BatchResponse
	Err      *Error
	// NotSupported reports that the plugin cannot run batches.
	NotSupported bool
}

var _ BatchSubstrate = (*PluginRPC)(nil)

// batchCallError maps a net/rpc error from a batch method: a plugin binary
// that predates the method answers "can't find method".
func batchCallError(method string, err error) error {
	var se rpc.ServerError
	if errors.As(err, &se) && strings.Contains(string(se), "can't find method") {
		return fmt.Errorf("%w: the plugin has no %s (%s)", ErrBatchNotSupported, method, string(se))
	}
	return err
}

func batchRespError(method string, notSupported bool, e *Error) error {
	switch {
	case notSupported && e != nil:
		return fmt.Errorf("%w: %s: %s", ErrBatchNotSupported, method, e.Diagnostic)
	case notSupported:
		return fmt.Errorf("%w: %s", ErrBatchNotSupported, method)
	case e != nil:
		return e
	default:
		return nil
	}
}

// CallBatch forwards the call
func (g *PluginRPC) CallBatch(tag string, requests []BatchRequestArgs, options *ConcreteRequestOptions) (*BatchResponse, error) {
	var resp RespCallBatch
	if err := g.client.Call("Plugin.CallBatch", &ArgsCallBatch{Tag: tag, Requests: requests, Options: options}, &resp); err != nil {
		return nil, batchCallError("CallBatch", err)
	}
	if err := batchRespError("CallBatch", resp.NotSupported, resp.Err); err != nil {
		return nil, err
	}
	return resp.Response, nil
}

// QueryBatch forwards the call
func (g *PluginRPC) QueryBatch(tag string, requests []BatchRequestArgs, options *ConcreteRequestOptions) (*BatchResponse, error) {
	var resp RespQueryBatch
	if err := g.client.Call("Plugin.QueryBatch", &ArgsQueryBatch{Tag: tag, Requests: requests, Options: options}, &resp); err != nil {
		return nil, batchCallError("QueryBatch", err)
	}
	if err := batchRespError("QueryBatch", resp.NotSupported, resp.Err); err != nil {
		return nil, err
	}
	return resp.Response, nil
}

// batch runs fn against the implementation's BatchSubstrate, reporting
// "not supported" when there is none or fn says so.
func (s *PluginRPCServer) batch(fn func(BatchSubstrate) (*BatchResponse, error)) (*BatchResponse, *Error, bool) {
	bs, ok := s.Impl.(BatchSubstrate)
	if !ok {
		return nil, &Error{Diagnostic: fmt.Sprintf("%T does not implement plugin.BatchSubstrate", s.Impl)}, true
	}
	res, err := fn(bs)
	if err != nil {
		return nil, s.newError(err), errors.Is(err, ErrBatchNotSupported)
	}
	return res, nil, false
}

// CallBatch forwards the call
func (s *PluginRPCServer) CallBatch(args *ArgsCallBatch, resp *RespCallBatch) error {
	resp.Response, resp.Err, resp.NotSupported = s.batch(func(bs BatchSubstrate) (*BatchResponse, error) {
		return bs.CallBatch(args.Tag, args.Requests, args.Options)
	})
	return nil
}

// QueryBatch forwards the call
func (s *PluginRPCServer) QueryBatch(args *ArgsQueryBatch, resp *RespQueryBatch) error {
	resp.Response, resp.Err, resp.NotSupported = s.batch(func(bs BatchSubstrate) (*BatchResponse, error) {
		return bs.QueryBatch(args.Tag, args.Requests, args.Options)
	})
	return nil
}
