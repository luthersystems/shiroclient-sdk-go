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

var (
	_ types.CallBatcher  = (*rpcShiroClient)(nil)
	_ types.QueryBatcher = (*rpcShiroClient)(nil)
)

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

// sendBatch validates and sends a batch through the gateway method named
// method (CallBatch or QueryBatch), which share their params, and returns
// the gateway's answer.  unsupported is the error a gateway without the
// method maps to.
func (c *rpcShiroClient) sendBatch(ctx context.Context, method string, unsupported error, requests []types.CallBatchRequest, configs []types.Config) (*rpcres, *types.RequestOptions, error) {
	name := "ShiroClient." + method
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return nil, nil, err
	}
	prepared, err := types.PrepareBatch(name, requests, opt)
	if err != nil {
		return nil, nil, err
	}
	reqs, err := batchRequestsJSON(prepared)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	params, err := callOptionParams(ctx, opt)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: shared %w", name, err)
	}
	params["requests"] = reqs

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  method,
		"params":  params,
	}
	res, err := c.reqres(ctx, req, opt)
	if err != nil {
		var rpcErr *jsonRPCError
		if errors.As(err, &rpcErr) && rpcErr.code == jsonRPCCodeMethodNotFound {
			return nil, nil, fmt.Errorf("%w: the gateway does not know %s (it needs luthersystems/substrate#521): %s",
				unsupported, method, rpcErr.message)
		}
		return nil, nil, err
	}
	switch res.errorLevel {
	case rpc.ErrorLevelNoError, rpc.ErrorLevelPhylum:
		return res, opt, nil
	case rpc.ErrorLevelShiroClient:
		return nil, nil, res.getShiroClientError()
	default:
		return nil, nil, fmt.Errorf("%s unexpected error level %d", name, res.errorLevel)
	}
}

// CallBatch implements types.CallBatcher: it runs requests as one
// all-or-nothing transaction through the gateway's CallBatch method.
func (c *rpcShiroClient) CallBatch(ctx context.Context, requests []types.CallBatchRequest, configs ...types.Config) (*types.CallBatchResponse, error) {
	ctx, span := c.tracer.Start(ctx, "sdk:CallBatch")
	defer span.End()
	return c.runBatch(ctx, rpc.MethodCallBatch, types.ErrCallBatchNotSupported, requests, configs)
}

// QueryBatch implements types.QueryBatcher: it simulates requests as one
// all-or-nothing transaction through the gateway's QueryBatch method, and
// never commits.
func (c *rpcShiroClient) QueryBatch(ctx context.Context, requests []types.CallBatchRequest, configs ...types.Config) (*types.CallBatchResponse, error) {
	ctx, span := c.tracer.Start(ctx, "sdk:QueryBatch")
	defer span.End()
	return c.runBatch(ctx, rpc.MethodQueryBatch, types.ErrQueryBatchNotSupported, requests, configs)
}

// runBatch sends a CallBatch or a QueryBatch and interprets the answer, which
// has the same shape for both; only a CallBatch may commit.
func (c *rpcShiroClient) runBatch(ctx context.Context, method string, unsupported error, requests []types.CallBatchRequest, configs []types.Config) (*types.CallBatchResponse, error) {
	name := "ShiroClient." + method
	query := method == rpc.MethodQueryBatch
	res, opt, err := c.sendBatch(ctx, method, unsupported, requests, configs)
	if err != nil {
		return nil, err
	}

	br, err := parseBatchResult(name, res, len(requests), !query)
	if err != nil {
		return nil, err
	}
	if query && br.Committed {
		return nil, fmt.Errorf("%s: gateway reported the batch committed; a QueryBatch never commits", name)
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
		return nil, fmt.Errorf("%s: gateway reported the batch committed (txid=%s) with a failed request", name, br.TxID)
	case res.errorLevel == rpc.ErrorLevelPhylum && failed < 0:
		return nil, fmt.Errorf("%s: batch not committed, but no request reported an error", name)
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

// batchRequestsJSON renders prepared requests as the gateway's "requests"
// parameter.
func batchRequestsJSON(requests []types.PreparedBatchRequest) ([]interface{}, error) {
	out := make([]interface{}, len(requests))
	for i, r := range requests {
		elem := map[string]interface{}{
			"method": r.Method,
			"params": r.Params,
		}
		if len(r.Transient) > 0 {
			transientJSON, err := encodeTransient(r.Transient)
			if err != nil {
				return nil, fmt.Errorf("request %d: %w", i, err)
			}
			elem["transient"] = transientJSON
		}
		if r.ID != nil {
			elem["id"] = r.ID
		}
		out[i] = elem
	}
	return out, nil
}

// parseBatchResult decodes a CallBatch or QueryBatch result: one Call-shaped
// result per request, plus the "committed" flag, which only a CallBatch
// result must carry.
func parseBatchResult(name string, res *rpcres, n int, needCommitted bool) (*types.CallBatchResponse, error) {
	elems, ok := res.result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s expected an array result field", name)
	}
	if len(elems) != n {
		return nil, fmt.Errorf("%s expected %d results, got %d", name, n, len(elems))
	}
	committed, ok := res.committed.(bool)
	if !ok && (needCommitted || res.committed != nil) {
		return nil, fmt.Errorf("%s expected a boolean committed field", name)
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
	} else if !needCommitted {
		// A QueryBatch commits nothing; it still reports its simulation.
		br.MaxSimBlockNum = res.simBlockNum
	}
	for i, elemArb := range elems {
		elem, ok := elemArb.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s expected an object for result %d", name, i)
		}
		br.IDs[i] = elem["id"]
		level, ok := elem["error_level"].(float64)
		if !ok {
			return nil, fmt.Errorf("%s expected a numeric error_level for result %d", name, i)
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
				return nil, fmt.Errorf("%s expected a numeric code for result %d", name, i)
			}
			message, ok := elem["message"].(string)
			if !ok {
				return nil, fmt.Errorf("%s expected a string message for result %d", name, i)
			}
			dataJSON, err := json.Marshal(elem["data"])
			if err != nil {
				return nil, err
			}
			br.Responses[i] = types.NewFailureResponse(int(code), message, dataJSON)
		default:
			return nil, fmt.Errorf("%s unexpected error level %d for result %d", name, int(level), i)
		}
	}
	return br, nil
}
