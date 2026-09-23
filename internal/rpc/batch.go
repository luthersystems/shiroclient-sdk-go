package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/rpc"
	"github.com/luthersystems/svc/txctx"
)

var _ types.BatchCaller = (*rpcShiroClient)(nil)

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

// CallBatch implements types.BatchCaller: it runs requests as one
// all-or-nothing transaction through the gateway's CallBatch method.
func (c *rpcShiroClient) CallBatch(ctx context.Context, requests []types.BatchRequest, configs ...types.Config) (*types.BatchResponse, error) {
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
	params := callOptionParams(ctx, opt)
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
				types.ErrBatchNotSupported, rpcErr.message)
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

	if br.Committed {
		txctx.SetTransactionDetails(ctx, txctx.TransactionDetails{TransactionID: br.TxID, CommitBlockNum: br.CommitBlockNum, MaxSimBlockNum: br.MaxSimBlockNum})
	}
	if opt.ResponseReceiver != nil {
		for _, r := range br.Responses {
			opt.ResponseReceiver(r)
		}
	}

	if res.errorLevel == rpc.ErrorLevelNoError {
		if failed := br.FailedIndex(); failed >= 0 {
			// A gateway never reports success for a batch with a failed
			// request; refuse to present one as committed if it does.
			return br, batchError(br, failed)
		}
		return br, nil
	}

	if br.Committed {
		return nil, errors.New("ShiroClient.CallBatch: a failed batch was reported committed")
	}
	failed := br.FailedIndex()
	if data, ok := res.data.(map[string]interface{}); ok {
		if idx, ok := data["failed_index"].(float64); ok && int(idx) >= 0 && int(idx) < len(br.Responses) {
			failed = int(idx)
		}
	}
	if failed < 0 {
		return br, errors.New("ShiroClient.CallBatch: batch not committed, but no request reported an error")
	}
	return br, batchError(br, failed)
}

func batchError(br *types.BatchResponse, failed int) error {
	return &types.BatchError{
		Index: failed,
		ID:    br.IDs[failed],
		Err:   br.Responses[failed].Error(),
	}
}

// batchRequestsJSON renders requests as the gateway's "requests" parameter.
func batchRequestsJSON(requests []types.BatchRequest) ([]interface{}, error) {
	if len(requests) == 0 {
		return nil, errors.New("ShiroClient.CallBatch: no requests")
	}
	out := make([]interface{}, len(requests))
	for i, r := range requests {
		if r.Method == "" {
			return nil, fmt.Errorf("ShiroClient.CallBatch: request %d has no method", i)
		}
		params := r.Params
		if params == nil {
			params = []interface{}{}
		}
		elem := map[string]interface{}{
			"method": r.Method,
			"params": params,
		}
		if r.ID != nil {
			elem["id"] = r.ID
		}
		out[i] = elem
	}
	return out, nil
}

// parseBatchResult decodes a CallBatch result: one Call-shaped result per
// request, plus the "committed" flag.
func parseBatchResult(res *rpcres, n int) (*types.BatchResponse, error) {
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
	br := &types.BatchResponse{
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
