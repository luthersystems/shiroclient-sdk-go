// Package shiroclient provides the ShiroClient interface and one
// implementations - a mode that connects to a JSON-RPC/HTTP gateway.
package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	//nolint:staticcheck // Deprecated package "github.com/golang/protobuf/jsonpb" used for backwards compatibility
	"github.com/golang/protobuf/jsonpb"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/types"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"
)

type ShiroClient = types.ShiroClient

type Config = types.Config

type RequestOptions = types.RequestOptions

type ShiroResponse = types.ShiroResponse

type Error = types.Error

type Transaction = types.Transaction

type Block = types.Block

var _ ShiroClient = (*rpcShiroClient)(nil)

// ProbeForCall returns the option values required for a call implied
// by an array of options.
func ProbeForCall(configs []Config) (context.Context, string, func(context.Context) string, logrus.Fields, string, string, interface{}, map[string][]byte, error) {
	rpc := &rpcShiroClient{baseConfig: nil, defaultLog: nil, httpClient: http.Client{}}
	ctx := context.TODO()
	ro, err := rpc.applyConfigs(ctx, configs...)
	if err != nil {
		return nil, "", nil, nil, "", "", nil, nil, err
	}
	return ro.Ctx, ro.ID, ro.TimestampGenerator, ro.LogFields, ro.AuthToken, ro.Creator, ro.Params, ro.Transient, nil
}

// ProbeForNew returns the option values required for creating a new
// mock implied by an array of options.
func ProbeForNew(configs []Config) (bool, *url.URL, error) {
	rpc := &rpcShiroClient{baseConfig: nil, defaultLog: nil, httpClient: http.Client{}}
	ctx := context.TODO()
	ro, err := rpc.applyConfigs(ctx, configs...)
	if err != nil {
		return false, nil, err
	}
	return ro.CcFetchURLDowngrade, ro.CcFetchURLProxy, nil
}

const (
	// MethodSeed is used to call the Seed method which re-opens a shiroclient.
	MethodSeed = "Seed"
	// MethodShiroPhylum is used to call the ShiroPhylum method which returns
	// an identifier for the current deployed phylum.
	MethodShiroPhylum = "ShiroPhylum"
	// MethodInit is used to call the Init method which initializes substrate.
	MethodInit = "Init"
	// MethodCall is used to call the Call method which executes a method on
	// the phylum.
	MethodCall = "Call"
	// MethodQueryInfo is used to call the QueryInfo method which returns the
	// blockchain height.
	MethodQueryInfo = "QueryInfo"
	// MethodQueryBlock is used to call the QueryBlock method which returns the
	// block information.
	MethodQueryBlock = "QueryBlock"
)

const (
	// ErrorLevelNoError indicates that no error occurred at any level
	ErrorLevelNoError = iota
	// ErrorLevelShiroClient indicates that an error occurred at the
	// ShiroClient level. That is, the invoked ShiroClient function
	// returned an error.
	ErrorLevelShiroClient
	// ErrorLevelPhylum indicates that the request was passed through to
	// the phylum successfully, but the phylum itself returned an error
	// response.
	ErrorLevelPhylum
)

const (
	// ErrorCodeShiroClientNone indicates no error code.
	ErrorCodeShiroClientNone = iota
	// ErrorCodeShiroClientTimeout indicates the shiro client timed out.
	ErrorCodeShiroClientTimeout
)

type rpcShiroClient struct {
	baseConfig []Config
	defaultLog *logrus.Logger
	httpClient http.Client
}

// rpcres is a type for a partially decoded RPC response.
type rpcres struct {
	errorLevel int
	result     interface{}
	code       interface{}
	message    interface{}
	data       interface{}
	txID       string
}

// scError wraps errors from shiroclient.
type scError struct {
	message string
	code    int
	err     error
}

// Unwrap implements the Wrapper interface from the errors package.
func (e *scError) Unwrap() error {
	return e.err
}

// Error implements error.
func (e *scError) Error() string {
	return e.message
}

// IsTimeoutError inspects an error returned from shiroclient and returns true
// if it's a timeout.
func IsTimeoutError(err error) bool {
	var se *scError
	if errors.As(err, &se) {
		return se.code == ErrorCodeShiroClientTimeout
	}
	return false
}

// Returns an error object with the same detail message as the
// ShiroClient error that was raised.
func (r *rpcres) getShiroClientError() error {
	message, ok := r.message.(string)
	if !ok {
		return &scError{
			message: "shiroclient error with no message",
		}
	}
	code, _ := r.code.(float64)
	return &scError{
		message: message,
		code:    int(code),
	}
}

func (c *rpcShiroClient) doRequest(ctx context.Context, httpReq *http.Request, log *logrus.Logger) ([]byte, error) {
	type result struct {
		msg []byte
		err error
	}
	resultCh := make(chan result, 1)

	go func() {
		httpRes, err := c.httpClient.Do(httpReq.WithContext(ctx))
		if err != nil {
			// just abort here, as the response.Body will already be closed
			// and you cannot drain a closed buffer.
			// from: https://cs.opensource.google/go/go/+/refs/tags/go1.20.6:src/net/http/client.go;l=581
			// On error, any Response can be ignored. A non-nil Response with a
			// non-nil error only occurs when CheckRedirect fails, and even then
			// the returned Response.Body is already closed.
			resultCh <- result{nil, err}
			return
		}

		msg, readErr := io.ReadAll(httpRes.Body)
		if readErr != nil {
			if log != nil {
				log.WithError(readErr).Warn("failed to read response body")
			}
			err = readErr
		}

		closeErr := httpRes.Body.Close()
		if closeErr != nil {
			if log != nil {
				log.WithError(closeErr).Warn("failed to close response body")
			}
			if err == nil {
				err = closeErr
			}
		}

		if err != nil {
			resultCh <- result{nil, err}
		} else {
			resultCh <- result{msg, nil}
		}
	}()

	select {
	case <-ctx.Done():
		// The context was canceled or the deadline exceeded, return context error
		// immediately, and leave the response cleanup to the goroutine.
		return nil, ctx.Err()
	case res := <-resultCh:
		err := res.err
		// The HTTP request finished.
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			// although unlikely, it's technically possible for the
			// resultChannel to return an error (e.g. EOF) due to the
			// cancelation, before the ctx.Done channel message is triggered.
			// Here, we wrap the non-canceled error as a canceled error, so
			// the application can properly handle it.
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, fmt.Errorf("%w: %s", context.Canceled, err)
			}
			return nil, err
		}
		return res.msg, nil
	}
}

// reqres is a round-trip "request/response" helper. Marshals "req",
// logs it at debug level, makes the HTTP request, reads and logs the
// response at debug level, unmarshals, parses into rpcres.
func (c *rpcShiroClient) reqres(req interface{}, opt *RequestOptions) (*rpcres, error) {
	outmsg, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if opt.Endpoint == "" {
		return nil, errors.New("ShiroClient.reqres expected an endpoint to be set")
	}

	ctx := opt.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	httpReq, err := http.NewRequest("POST", opt.Endpoint, bytes.NewReader(outmsg))
	if err != nil {
		return nil, err
	}

	for k, v := range opt.Headers {
		httpReq.Header.Set(k, v)
	}
	if opt.AuthToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+opt.AuthToken)
	}

	msg, err := c.doRequest(ctx, httpReq, opt.Log)
	if err != nil {
		return nil, fmt.Errorf("ShiroClient.reqres: %w", err)
	}

	var target *interface{}

	if opt.Target == nil {
		var resArb interface{}
		target = &resArb
	} else {
		target = opt.Target
	}

	err = json.Unmarshal(msg, target)
	if err != nil {
		return nil, err
	}

	resArb := *target

	resCurly, ok := resArb.(map[string]interface{})
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected an object")
	}

	jsonrpcArb, ok := resCurly["jsonrpc"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a jsonrpc field")
	}

	jsonrpc, ok := jsonrpcArb.(string)
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a string jsonrpc field")
	}

	if jsonrpc != "2.0" {
		return nil, errors.New("ShiroClient.reqres expected jsonrpc version 2.0")
	}

	resultArb, ok := resCurly["result"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a result field")
	}

	resultCurly, ok := resultArb.(map[string]interface{})
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected an object result field")
	}

	errorLevelArb, ok := resultCurly["error_level"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected an error_level field")
	}

	errorLevel, ok := errorLevelArb.(float64)
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a numeric error_level field")
	}

	result, ok := resultCurly["result"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a result field")
	}

	code, ok := resultCurly["code"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a code field")
	}

	message, ok := resultCurly["message"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a message field")
	}

	data, ok := resultCurly["data"]
	if !ok {
		return nil, errors.New("ShiroClient.reqres expected a data field")
	}

	// $transaction_id appears on some requests
	txID, _ := resCurly["$commit_tx_id"].(string)

	return &rpcres{
		errorLevel: int(errorLevel),
		result:     result,
		code:       code,
		message:    message,
		data:       data,
		txID:       txID,
	}, nil
}

// applyConfigs applies configs -- baseConfigs supplied in the
// constructor first, followed by configs arguments.
func (c *rpcShiroClient) applyConfigs(ctx context.Context, configs ...Config) (*RequestOptions, error) {
	tConfigs := make([]Config, len(c.baseConfig)+len(configs))
	tConfigs = append(tConfigs, append(c.baseConfig, configs...)...)
	return types.ApplyConfigs(ctx, c.defaultLog, tConfigs...), nil
}

// HealthCheck uses the RPC gateway server's health endpoint to check
// connectivity to the gateway itself and any specified upstream services.
// HealthCheck is not part of the ShiroClient interface but it is recognized by
// the RemoteHealthCheck function.
func (c *rpcShiroClient) HealthCheck(ctx context.Context, services []string, configs ...Config) (HealthCheck, error) {
	// Validate config and transform params
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return nil, fmt.Errorf("healthcheck config: %w", err)
	}
	if opt.Endpoint == "" {
		return nil, errors.New("ShiroClient.HealthCheck expected an endpoint to be set")
	}
	checkURL, err := gatewayHealthCheckURL(opt.Endpoint, services)
	if err != nil {
		return nil, fmt.Errorf("healthcheck invalid endpoint: %w", err)
	}

	// Do the health check
	hreq, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return nil, fmt.Errorf("healthcheck request: %w", err)
	}

	body, err := c.doRequest(ctx, hreq, c.defaultLog)
	if err != nil {
		return nil, fmt.Errorf("healthcheck perform: %w", err)
	}

	resp, err := unmarshalHealthResponse(body)
	if err != nil {
		return nil, fmt.Errorf("healthcheck bad response: %w", err)
	}

	// resp should not contain an exception
	return resp, nil
}

func gatewayHealthCheckURL(endpoint string, services []string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid gateway url: %w", err)
	}
	u.Path = path.Join(u.Path, "health_check")
	_, err = url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid gateway query parameters: %w", err)
	}
	if len(services) > 0 {
		urlQueryAppend(u, url.Values{"service": services})
	}
	return u.String(), nil
}

// urlQueryAppend modifies u, appending a set of key-value pairs to its query.
// urlQueryAppend attempts to avoid rearranging previously existing query
// parameters in u.
func urlQueryAppend(u *url.URL, vals url.Values) {
	// Semi-hacky append of additional (healthcheck) query params a url which
	// may already contain a query string.  Attempting to parse the query can
	// be a lossy conversion in the case of malformed input.
	paramStr := vals.Encode()
	switch {
	case u.RawQuery == "":
		u.RawQuery = paramStr
	default:
		u.RawQuery += "&" + paramStr
	}
}

// Seed implements the ShiroClient interface.
func (c *rpcShiroClient) Seed(version string, configs ...Config) error {
	ctx := context.TODO()
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodSeed,
		"params": map[string]interface{}{
			"version": version,
		},
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		return nil

	case ErrorLevelShiroClient:
		return res.getShiroClientError()

	default:
		return fmt.Errorf("ShiroClient.Seed unexpected error level %d", res.errorLevel)
	}
}

// ShiroPhylum implements the ShiroClient interface.
func (c *rpcShiroClient) ShiroPhylum(configs ...Config) (string, error) {
	ctx := context.TODO()
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return "", err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodShiroPhylum,
		"params":  map[string]interface{}{},
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return "", err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		res, ok := res.result.(string)
		if !ok {
			return "", errors.New("ShiroClient.ShiroPhylum expected string result field")
		}

		return res, nil

	case ErrorLevelShiroClient:
		return "", res.getShiroClientError()

	default:
		return "", fmt.Errorf("ShiroClient.ShiroPhylum unexpected error level %d", res.errorLevel)
	}
}

// Init implements the ShiroClient interface.
func (c *rpcShiroClient) Init(phylum string, configs ...Config) error {
	ctx := context.TODO()
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodInit,
		"params": map[string]interface{}{
			"phylum": phylum,
		},
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		return nil

	case ErrorLevelShiroClient:
		return res.getShiroClientError()

	default:
		return fmt.Errorf("ShiroClient.Init unexpected error level %d", res.errorLevel)
	}
}

// Call implements the ShiroClient interface.
func (c *rpcShiroClient) Call(ctx context.Context, method string, configs ...Config) (ShiroResponse, error) {
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return nil, err
	}

	transientJSON := make(map[string]interface{})

	for k, v := range opt.Transient {
		transientJSON[k] = hex.EncodeToString(v)
	}

	if opt.TimestampGenerator != nil {
		transientJSON["timestamp_override"] = hex.EncodeToString([]byte(opt.TimestampGenerator(ctx)))
	}

	params := map[string]interface{}{
		"method":    method,
		"params":    opt.Params,
		"transient": transientJSON,
	}
	if opt.DependentTxID != "" {
		params["dependent_txid"] = opt.DependentTxID
	}
	if opt.DisableWritePolling {
		params["disable_write_polling"] = opt.DisableWritePolling
	}
	params["cc_fetchurl_downgrade"] = opt.CcFetchURLDowngrade
	if opt.CcFetchURLProxy != nil {
		params["cc_fetchurl_proxy"] = opt.CcFetchURLProxy.String()
	} else {
		params["cc_fetchurl_proxy"] = ""
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodCall,
		"params":  params,
	}

	if len(opt.MspFilter) > 0 {
		req["params"].(map[string]interface{})["msp_filter"] = opt.MspFilter
	}

	if opt.MinEndorsers > 0 {
		req["params"].(map[string]interface{})["min_endorsers"] = opt.MinEndorsers
	}

	if opt.Creator != "" {
		req["params"].(map[string]interface{})["creator_msp_id"] = opt.Creator
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return nil, err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		resultJSON, err := json.Marshal(res.result)
		if err != nil {
			return nil, err
		}

		return types.NewSuccessResponse(resultJSON, res.txID), nil

	case ErrorLevelShiroClient:
		return nil, res.getShiroClientError()

	case ErrorLevelPhylum:
		dataJSON, err := json.Marshal(res.data)
		if err != nil {
			return nil, err
		}

		code, ok := res.code.(float64)
		if !ok {
			return nil, errors.New("ShiroClient.Call expected a numeric code field")
		}

		message, ok := res.message.(string)
		if !ok {
			return nil, errors.New("ShiroClient.Call expected a string message field")
		}

		return types.NewFailureResponse(int(code), message, dataJSON), nil

	default:
		return nil, fmt.Errorf("ShiroClient.Call unexpected error level %d", res.errorLevel)
	}
}

// QueryInfo implements the ShiroClient interface.
func (c *rpcShiroClient) QueryInfo(configs ...Config) (uint64, error) {
	ctx := context.TODO()
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return 0, err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodQueryInfo,
		"params":  map[string]interface{}{},
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return 0, err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		height, ok := res.result.(float64)
		if !ok {
			return 0, errors.New("ShiroClient.QueryInfo expected a numeric result field")
		}

		return uint64(height), nil

	case ErrorLevelShiroClient:
		return 0, res.getShiroClientError()

	default:
		return 0, fmt.Errorf("ShiroClient.QueryInfo unexpected error level %d", res.errorLevel)
	}
}

// QueryBlock implements the ShiroClient interface.
func (c *rpcShiroClient) QueryBlock(blockNumber uint64, configs ...Config) (Block, error) {
	ctx := context.TODO()
	opt, err := c.applyConfigs(ctx, configs...)
	if err != nil {
		return nil, err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.ID,
		"method":  MethodQueryBlock,
		"params":  map[string]interface{}{"block_number": float64(blockNumber)},
	}

	res, err := c.reqres(req, opt)
	if err != nil {
		return nil, err
	}

	switch res.errorLevel {
	case ErrorLevelNoError:
		res, ok := res.result.(map[string]interface{})
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected an object result field")
		}

		blockHashArb, ok := res["block_hash"]
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a block_hash field")
		}

		blockHash, ok := blockHashArb.(string)
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a string block_hash field")
		}

		// transaction IDs

		txidsArb, ok := res["transaction_ids"]
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a transaction_ids field")
		}

		txids, ok := txidsArb.([]interface{})
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected an array transaction_ids field")
		}

		txidsOut := make([]string, len(txids))

		for idx, txidArb := range txids {
			txid, ok := txidArb.(string)
			if !ok {
				return nil, errors.New("ShiroClient.QueryBlock expected a string transaction_id member")
			}

			txidsOut[idx] = txid
		}

		// reasons

		reasonsArb, ok := res["transaction_reasons"]
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a transaction_reasons field")
		}

		reasons, ok := reasonsArb.([]interface{})
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected an array transaction_reasons field")
		}

		reasonsOut := make([]string, len(reasons))

		for idx, reasonArb := range reasons {
			reason, ok := reasonArb.(string)
			if !ok {
				return nil, errors.New("ShiroClient.QueryBlock expected a string transaction_reason member")
			}

			reasonsOut[idx] = reason
		}

		// events

		eventsArb, ok := res["transaction_events"]
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a transaction_events field")
		}

		events, ok := eventsArb.([]interface{})
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected an array transaction_events field")
		}

		eventsOut := make([][]byte, len(events))

		for idx, eventArb := range events {
			event, ok := eventArb.(string)
			if !ok {
				return nil, errors.New("ShiroClient.QueryBlock expected a string transaction_event member")
			}

			eventBytes, err := base64.StdEncoding.DecodeString(event)
			if err != nil {
				return nil, errors.New("ShiroClient.QueryBlock expected a base64 string transaction_event member")
			}
			eventsOut[idx] = eventBytes
		}

		// chaincode IDs

		ccidsArb, ok := res["chaincode_ids"]
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected a chaincode_ids field")
		}

		ccids, ok := ccidsArb.([]interface{})
		if !ok {
			return nil, errors.New("ShiroClient.QueryBlock expected an array chaincode_ids field")
		}

		ccidsOut := make([]string, len(ccids))

		for idx, ccidsArb := range ccids {
			ccid, ok := ccidsArb.(string)
			if !ok {
				return nil, errors.New("ShiroClient.QueryBlock expected a string chaincode_id member")
			}

			ccidsOut[idx] = ccid
		}

		// build transactions

		transactions := make([]Transaction, len(txidsOut))

		if len(txidsOut) != len(reasonsOut) {
			return nil, errors.New("ShiroClient.QueryBlock: mismatched parallel arrays")
		}

		for i, txid := range txidsOut {
			transactions[i] = types.NewTransaction(txid, reasonsOut[i], eventsOut[i], ccidsOut[i])
		}

		return types.NewBlock(blockHash, transactions), nil

	case ErrorLevelShiroClient:
		return nil, res.getShiroClientError()

	default:
		return nil, fmt.Errorf("ShiroClient.QueryBlock unexpected error level %d", res.errorLevel)
	}
}

// NewRPC creates a new RPC ShiroClient with the given set of base
// configs that will be applied to all commands.
func NewRPC(clientConfigs []Config) ShiroClient {
	return &rpcShiroClient{
		baseConfig: clientConfigs,
		defaultLog: logrus.New(),
		httpClient: http.Client{},
	}
}

// UnmarshalProto attempts to unmarshal protobuf bytes with backwards compatability.
func UnmarshalProto(src []byte, dst interface{}) error {
	var err error
	switch message := dst.(type) {
	case proto.Message:
		err = protojson.Unmarshal(src, message)
	case protoiface.MessageV1:
		//nolint:staticcheck // Deprecated Unmarshal used for backwards compatibility
		err = jsonpb.Unmarshal(bytes.NewReader(src), message)
	default:
		err = json.Unmarshal(src, message)
	}
	if err != nil {
		return err
	}

	return nil
}
