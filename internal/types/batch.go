package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrCallBatchNotSupported is returned by CallBatch when the client or the
// gateway cannot run a batch: a gateway older than luthersystems/substrate#521
// answers "method not found", and the mock plugin does not support batches
// yet.  Nothing was run.
var ErrCallBatchNotSupported = errors.New("shiroclient: CallBatch not supported")

// CallBatcher is implemented by clients that can run several phylum methods
// as one all-or-nothing transaction.  It is separate from ShiroClient so that
// adding it did not break other implementations of that interface.
type CallBatcher interface {
	CallBatch(ctx context.Context, requests []CallBatchRequest, config ...Config) (*CallBatchResponse, error)
}

// CallBatchRequest is one request of a CallBatch.
type CallBatchRequest struct {
	// Params are the method's parameters, which must encode to a JSON array
	// or object.  Anything that encodes to null (nil, or a nil slice, map or
	// pointer) is sent as an empty array; any other value is rejected before
	// the batch is sent.
	Params interface{}
	// ID is the request's JSON-RPC id: nil, a string or a number (a Go
	// integer or float type, or json.Number).  Other types are rejected
	// before the batch is sent.  When nil, the server uses the request's
	// index in the batch.
	ID interface{}
	// Method is the phylum endpoint to call.
	Method string
}

// CallBatchResponse is the result of a CallBatch.
type CallBatchResponse struct {
	// Responses holds one response per request, in request order.  In a
	// batch that was not committed because a request failed, every response
	// is a failure.
	Responses []ShiroResponse
	// IDs holds each response's JSON-RPC id as the server returned it,
	// decoded from JSON (so a numeric id is a float64).
	IDs []interface{}
	// TxID is the committed transaction's ID; empty when not committed.
	TxID string
	// CommitBlockNum is the block that committed the transaction, or 0.
	CommitBlockNum uint64
	// MaxSimBlockNum is the max block number that simulated the batch.
	MaxSimBlockNum uint64
	// Committed reports whether the batch was committed: it wrote state and
	// no request failed.  A batch that only read is not committed.
	Committed bool
}

// FailedIndex returns the index of the request whose own failure spoiled the
// batch, or -1 when no request failed.  When no failure is attributable to a
// single request, the first failed request is returned.
func (r *CallBatchResponse) FailedIndex() int {
	if r == nil {
		return -1
	}
	first := -1
	for i, resp := range r.Responses {
		if resp == nil || resp.Error() == nil {
			continue
		}
		if first < 0 {
			first = i
		}
		if _, aborted := CallBatchAborted(resp.Error()); !aborted {
			return i
		}
	}
	return first
}

// CallBatchError is returned, together with the CallBatchResponse, when a request of
// a batch failed and so nothing was committed.
type CallBatchError struct {
	// ID is the failed request's JSON-RPC id, as in CallBatchResponse.IDs.
	ID interface{}
	// Err is the failed request's own error.
	Err Error
	// Index is the failed request's index in the batch.
	Index int
}

// Error implements error.
func (e *CallBatchError) Error() string {
	msg := "unknown error"
	if e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("shiroclient: batch not committed: request %d (id %v) failed: %s", e.Index, e.ID, msg)
}

// Unwrap returns the failed request's error.
func (e *CallBatchError) Unwrap() error {
	if e.Err == nil {
		return nil
	}
	return e.Err
}

// CodeBatchAborted is the JSON-RPC error code substrate reserves for a request
// of a batch that was not run, or not committed, because another request
// failed. The router never lets a phylum error carry it.
const CodeBatchAborted = -32001

// CallBatchAborted reports whether err is the error given to a request of a batch
// that did not fail itself but was not committed because another request
// failed.  failedID is the id of the request that failed. Only the reserved
// code CodeBatchAborted counts: a phylum error whose data merely looks like
// the abort marker is that request's own failure.
func CallBatchAborted(err Error) (failedID interface{}, ok bool) {
	if err == nil || err.Code() != CodeBatchAborted {
		return nil, false
	}
	var data struct {
		FailedID         interface{} `json:"failed_id"`
		CallBatchAborted bool        `json:"batch_aborted"`
	}
	if json.Unmarshal(err.DataJSON(), &data) != nil || !data.CallBatchAborted {
		return nil, false
	}
	return data.FailedID, true
}
