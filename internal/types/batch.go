package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrBatchNotSupported is returned by CallBatch when the client or the
// gateway cannot run a batch: a gateway older than luthersystems/substrate#521
// answers "method not found", and the mock plugin does not support batches
// yet.  Nothing was run.
var ErrBatchNotSupported = errors.New("shiroclient: CallBatch not supported")

// BatchCaller is implemented by clients that can run several phylum methods
// as one all-or-nothing transaction.  It is separate from ShiroClient so that
// adding it did not break other implementations of that interface.
type BatchCaller interface {
	CallBatch(ctx context.Context, requests []BatchRequest, config ...Config) (*BatchResponse, error)
}

// BatchRequest is one request of a CallBatch.
type BatchRequest struct {
	// Params are the method's parameters: an array or an object, as for
	// Call's WithParams.  Nil is sent as an empty array.
	Params interface{}
	// ID is the request's JSON-RPC id.  When nil, the server uses the
	// request's index in the batch.
	ID interface{}
	// Method is the phylum endpoint to call.
	Method string
}

// BatchResponse is the result of a CallBatch.
type BatchResponse struct {
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
func (r *BatchResponse) FailedIndex() int {
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
		if _, aborted := BatchAborted(resp.Error()); !aborted {
			return i
		}
	}
	return first
}

// BatchError is returned, together with the BatchResponse, when a request of
// a batch failed and so nothing was committed.
type BatchError struct {
	// ID is the failed request's JSON-RPC id, as in BatchResponse.IDs.
	ID interface{}
	// Err is the failed request's own error.
	Err Error
	// Index is the failed request's index in the batch.
	Index int
}

// Error implements error.
func (e *BatchError) Error() string {
	msg := "unknown error"
	if e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("shiroclient: batch not committed: request %d (id %v) failed: %s", e.Index, e.ID, msg)
}

// Unwrap returns the failed request's error.
func (e *BatchError) Unwrap() error {
	if e.Err == nil {
		return nil
	}
	return e.Err
}

// BatchAborted reports whether err is the error given to a request of a batch
// that did not fail itself but was not committed because another request
// failed.  failedID is the id of the request that failed.
func BatchAborted(err Error) (failedID interface{}, ok bool) {
	if err == nil {
		return nil, false
	}
	var data struct {
		FailedID     interface{} `json:"failed_id"`
		BatchAborted bool        `json:"batch_aborted"`
	}
	if json.Unmarshal(err.DataJSON(), &data) != nil || !data.BatchAborted {
		return nil, false
	}
	return data.FailedID, true
}
