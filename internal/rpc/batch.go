package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/rpc"
	"github.com/luthersystems/svc/txctx"
)

var _ types.CallBatcher = (*rpcShiroClient)(nil)

// jsonRPCCodeMethodNotFound is the JSON-RPC 2.0 "method not found" code.
const jsonRPCCodeMethodNotFound = -32601

// jsonRPCError is a JSON-RPC 2.0 error object: the gateway's answer to a
// request it rejected before running it (unknown method, invalid params).
type jsonRPCError struct {
	message string
	code    int
}

// Error implements error.
func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.code, e.message)
}

// jsonRPCErrorOf decodes a JSON-RPC error object, or returns nil.
func jsonRPCErrorOf(arb interface{}) *jsonRPCError {
	obj, ok := arb.(map[string]interface{})
	if !ok {
		return nil
	}
	code, ok := obj["code"].(float64)
	if !ok {
		return nil
	}
	message, _ := obj["message"].(string)
	return &jsonRPCError{code: int(code), message: message}
}

// CallBatch implements types.CallBatcher: it runs requests as one
// all-or-nothing transaction through the gateway's CallBatch method.
func (c *rpcShiroClient) CallBatch(ctx context.Context, requests []types.CallBatchRequest, configs ...types.Config) (*types.CallBatchResponse, error) {
	ctx, span := c.tracer.Start(ctx, "sdk:CallBatch")
	defer span.End()
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return nil, err
	}

	reqs, err := batchRequestsJSON(requests)
	if err != nil {
		return nil, err
	}
	params, err := callOptionParams(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("ShiroClient.CallBatch: shared %w", err)
	}
	params["requests"] = reqs

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  rpc.MethodCallBatch,
		"params":  params,
	}

	res, err := c.reqres(ctx, req, opt)
	if err != nil {
		var rpcErr *jsonRPCError
		if errors.As(err, &rpcErr) && rpcErr.code == jsonRPCCodeMethodNotFound {
			return nil, fmt.Errorf("%w: the gateway does not know CallBatch (it needs luthersystems/substrate#521): %s",
				types.ErrCallBatchNotSupported, rpcErr.message)
		}
		return nil, err
	}

	switch res.errorLevel {
	case rpc.ErrorLevelNoError, rpc.ErrorLevelPhylum:
	case rpc.ErrorLevelShiroClient:
		return nil, res.getShiroClientError()
	default:
		return nil, fmt.Errorf("ShiroClient.CallBatch unexpected error level %d", res.errorLevel)
	}

	br, err := parseBatchResult(res, len(requests))
	if err != nil {
		return nil, err
	}

	failed := br.FailedIndex()
	if res.errorLevel == rpc.ErrorLevelPhylum {
		if data, ok := res.data.(map[string]interface{}); ok {
			if idx, ok := data["failed_index"].(float64); ok && int(idx) >= 0 && int(idx) < len(br.Responses) {
				failed = int(idx)
			}
		}
	}
	switch {
	case br.Committed && (failed >= 0 || res.errorLevel != rpc.ErrorLevelNoError):
		// A committed batch has no failed request.  Neither success nor a
		// clean failure can be claimed from a contradictory answer.
		return nil, fmt.Errorf("ShiroClient.CallBatch: gateway reported the batch committed (txid=%s) with a failed request", br.TxID)
	case res.errorLevel == rpc.ErrorLevelPhylum && failed < 0:
		return nil, errors.New("ShiroClient.CallBatch: batch not committed, but no request reported an error")
	}

	// Every check passed: publish the result.
	if br.Committed {
		txctx.SetTransactionDetails(ctx, txctx.TransactionDetails{TransactionID: br.TxID, CommitBlockNum: br.CommitBlockNum, MaxSimBlockNum: br.MaxSimBlockNum})
	}
	if opt.ResponseReceiver != nil {
		for _, r := range br.Responses {
			opt.ResponseReceiver(r)
		}
	}
	if failed >= 0 {
		return br, batchError(br, failed)
	}
	return br, nil
}

func batchError(br *types.CallBatchResponse, failed int) error {
	return &types.CallBatchError{
		Index: failed,
		ID:    br.IDs[failed],
		Err:   br.Responses[failed].Error(),
	}
}

// batchRequestsJSON renders requests as the gateway's "requests" parameter,
// rejecting what the gateway would reject for the whole batch.
func batchRequestsJSON(requests []types.CallBatchRequest) ([]interface{}, error) {
	if len(requests) == 0 {
		return nil, errors.New("ShiroClient.CallBatch: no requests")
	}
	out := make([]interface{}, len(requests))
	for i, r := range requests {
		if r.Method == "" {
			return nil, fmt.Errorf("ShiroClient.CallBatch: request %d has no method", i)
		}
		params, err := batchParamsJSON(r.Params)
		if err != nil {
			return nil, fmt.Errorf("ShiroClient.CallBatch: request %d: %w", i, err)
		}
		elem := map[string]interface{}{
			"method": r.Method,
			"params": params,
		}
		if len(r.Transient) > 0 {
			for k := range r.Transient {
				if k == "" {
					return nil, fmt.Errorf("ShiroClient.CallBatch: request %d: empty transient key", i)
				}
			}
			transient, err := encodeTransient(r.Transient)
			if err != nil {
				return nil, fmt.Errorf("ShiroClient.CallBatch: request %d: %w", i, err)
			}
			elem["transient"] = transient
		}
		if r.ID != nil {
			if !validBatchID(r.ID) {
				return nil, fmt.Errorf("ShiroClient.CallBatch: request %d: id must be a string, or a number within ±2^53 (the gateway decodes ids as float64); got %T %v", i, r.ID, r.ID)
			}
			elem["id"] = r.ID
		}
		out[i] = elem
	}
	return out, nil
}

// batchParamsJSON encodes a request's params.  The gateway accepts only an
// array or an object; anything that encodes to null (nil, or a typed nil
// slice, map or pointer) is sent as an empty array.
func batchParamsJSON(params interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("params: %w", err)
	}
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("null")):
		return json.RawMessage("[]"), nil
	case len(b) > 0 && (b[0] == '[' || b[0] == '{'):
		return json.RawMessage(b), nil
	default:
		return nil, fmt.Errorf("params must encode to an array or an object, not %s", b)
	}
}

// validBatchID reports whether id is a JSON-RPC id the gateway echoes: a
// string or a number.
func validBatchID(id interface{}) bool {
	if n, ok := id.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i >= -maxExactBatchID && i <= maxExactBatchID
		}
		f, err := n.Float64()
		return err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	}
	// Kinds, not exact types, so a caller's own id type (type OrderID
	// string) is accepted. Integers are limited to +/-2^53 because the
	// gateway decodes ids as float64: a larger id would come back changed.
	v := reflect.ValueOf(id)
	switch v.Kind() {
	case reflect.String:
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() >= -maxExactBatchID && v.Int() <= maxExactBatchID
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() <= maxExactBatchID
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		return !math.IsInf(f, 0) && !math.IsNaN(f)
	default:
		return false
	}
}

// maxExactBatchID is the largest integer a float64 holds exactly (2^53).
const maxExactBatchID = 1 << 53

// parseBatchResult decodes a CallBatch result: one Call-shaped result per
// request, plus the "committed" flag.
func parseBatchResult(res *rpcres, n int) (*types.CallBatchResponse, error) {
	elems, ok := res.result.([]interface{})
	if !ok {
		return nil, errors.New("ShiroClient.CallBatch expected an array result field")
	}
	if len(elems) != n {
		return nil, fmt.Errorf("ShiroClient.CallBatch expected %d results, got %d", n, len(elems))
	}
	committed, ok := res.committed.(bool)
	if !ok {
		return nil, errors.New("ShiroClient.CallBatch expected a boolean committed field")
	}
	br := &types.CallBatchResponse{
		Responses: make([]types.ShiroResponse, n),
		IDs:       make([]interface{}, n),
		Committed: committed,
	}
	if committed {
		br.TxID = res.txID
		br.CommitBlockNum = res.comBlockNum
		br.MaxSimBlockNum = res.simBlockNum
	}
	for i, elemArb := range elems {
		elem, ok := elemArb.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("ShiroClient.CallBatch expected an object for result %d", i)
		}
		br.IDs[i] = elem["id"]
		level, ok := elem["error_level"].(float64)
		if !ok {
			return nil, fmt.Errorf("ShiroClient.CallBatch expected a numeric error_level for result %d", i)
		}
		switch int(level) {
		case rpc.ErrorLevelNoError:
			resultJSON, err := json.Marshal(elem["result"])
			if err != nil {
				return nil, err
			}
			br.Responses[i] = types.NewSuccessResponse(resultJSON, br.TxID, br.CommitBlockNum, br.MaxSimBlockNum)
		case rpc.ErrorLevelPhylum:
			code, ok := elem["code"].(float64)
			if !ok {
				return nil, fmt.Errorf("ShiroClient.CallBatch expected a numeric code for result %d", i)
			}
			message, ok := elem["message"].(string)
			if !ok {
				return nil, fmt.Errorf("ShiroClient.CallBatch expected a string message for result %d", i)
			}
			dataJSON, err := json.Marshal(elem["data"])
			if err != nil {
				return nil, err
			}
			br.Responses[i] = types.NewFailureResponse(int(code), message, dataJSON)
		default:
			return nil, fmt.Errorf("ShiroClient.CallBatch unexpected error level %d for result %d", int(level), i)
		}
	}
	return br, nil
}
