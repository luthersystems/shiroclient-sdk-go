// Package shiroclient provides the ShiroClient interface and one
// implementations - a mode that connects to a JSON-RPC/HTTP gateway.
package shiroclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/luthersystems/substratecommon"
	"github.com/sirupsen/logrus"
)

// requestOptions are operated on by the Config functions generated by
// the With* functions. There is no need for a consumer of this
// library to directly manipulate objects of this type.
type requestOptions struct {
	log                 *logrus.Logger
	logFields           logrus.Fields
	headers             map[string]string
	endpoint            string
	id                  string
	authToken           string
	params              interface{}
	transient           map[string][]byte
	target              *interface{}
	timestampGenerator  func(context.Context) string
	mspFilter           []string
	minEndorsers        int
	creator             string
	ctx                 context.Context
	dependentTxID       string
	disableWritePolling bool
	ccFetchURLDowngrade bool
	ccFetchURLProxy     *url.URL
}

// Config is a type for a function that can mutate a requestOptions
// object.
type Config func(*requestOptions)

// WithContext allows specifying the context to use.
func WithContext(ctx context.Context) Config {
	return func(r *requestOptions) {
		r.ctx = ctx
	}
}

// WithLog allows specifying the logger to use.
func WithLog(log *logrus.Logger) Config {
	return func(r *requestOptions) {
		r.log = log
	}
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return func(r *requestOptions) {
		r.logFields[key] = value
	}
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return func(r *requestOptions) {
		for k, v := range fields {
			r.logFields[k] = v
		}
	}
}

// WithHeader allows specifying an additional HTTP header.
func WithHeader(key string, value string) Config {
	return func(r *requestOptions) {
		r.headers[key] = value
	}
}

// WithEndpoint allows specifying the endpoint to target. The RPC
// implementation will not work if an endpoint is not specified.
func WithEndpoint(endpoint string) Config {
	return func(r *requestOptions) {
		r.endpoint = endpoint
	}
}

// WithID allows specifying the request ID. If the request ID is not
// specified, a randomly-generated UUID will be used.
func WithID(id string) Config {
	return func(r *requestOptions) {
		r.id = id
	}
}

// WithParams allows specifying the phylum "parameters" argument. This
// must be set to something that json.Marshal accepts.
func WithParams(params interface{}) Config {
	return func(r *requestOptions) {
		r.params = params
	}
}

// WithTransientData allows specifying a single "transient data"
// key-value pair.
func WithTransientData(key string, val []byte) Config {
	return func(r *requestOptions) {
		r.transient[key] = val
	}
}

// WithTransientDataMap allows specifying multiple "transient data"
// key-value pairs.
func WithTransientDataMap(data map[string][]byte) Config {
	return func(r *requestOptions) {
		for key, val := range data {
			r.transient[key] = val
		}
	}
}

// WithResponse allows capturing the RPC response for futher analysis.
func WithResponse(target *interface{}) Config {
	return func(r *requestOptions) {
		r.target = target
	}
}

// WithAuthToken passes authorization for the transaction issuer with a request
func WithAuthToken(token string) Config {
	return func(r *requestOptions) {
		r.authToken = token
	}
}

// WithTimestampGenerator allows specifying a function that will be
// invoked at every Init or Call whose output is used to set the
// substrate "now" timestamp in mock mode. Has no effect outside of
// mock mode.
func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return func(r *requestOptions) {
		r.timestampGenerator = timestampGenerator
	}
}

// WithMSPFilter allows specifying the MSP filter. Has no effect in
// mock mode.
func WithMSPFilter(mspFilter []string) Config {
	clonedMSPFilter := append([]string(nil), mspFilter...)
	return (func(r *requestOptions) {
		r.mspFilter = clonedMSPFilter
	})
}

// WithMinEndorsers allows specifying the minimum number of endorsing
// peers. Has no effect in mock mode.
func WithMinEndorsers(minEndorsers int) Config {
	return (func(r *requestOptions) {
		r.minEndorsers = minEndorsers
	})
}

// WithCreator allows specifying the creator. Only has effect in mock
// mode. Also works in gateway mock mode.
func WithCreator(creator string) Config {
	return (func(r *requestOptions) {
		r.creator = creator
	})
}

// WithDependentTxID allows specifying a dependency on a transaction ID.  If
// set, the client will poll for the presence of that transaction before
// simulating the request on the peer with the transaction.
func WithDependentTxID(txID string) Config {
	return (func(r *requestOptions) {
		r.dependentTxID = txID
	})
}

// WithDisableWritePolling allows disabling polling for full consensus after a
// write is committed.
func WithDisableWritePolling(disable bool) Config {
	return (func(r *requestOptions) {
		r.disableWritePolling = disable
	})
}

// WithCCFetchURLDowngrade allows controlling https -> http downgrade,
// typically useful before proxying for ccfetchurl library.
func WithCCFetchURLDowngrade(downgrade bool) Config {
	return (func(r *requestOptions) {
		r.ccFetchURLDowngrade = downgrade
	})
}

// WithCCFetchURLProxy sets the proxy for ccfetchurl library.
func WithCCFetchURLProxy(proxy *url.URL) Config {
	return (func(r *requestOptions) {
		r.ccFetchURLProxy = proxy
	})
}

// ProbeTimestampGenerator returns the timestamp generator implied by
// an array of options.
func ProbeTimestampGenerator(configs []Config) (func(context.Context) string, error) {
	rpc := &rpcShiroClient{baseConfig: nil, defaultLog: nil, httpClient: http.Client{}}
	ro, err := rpc.applyConfigs(configs...)
	if err != nil {
		return nil, err
	}
	return ro.timestampGenerator, nil
}

// ProbeForCall returns the option values required for a call implied
// by an array of options.
func ProbeForCall(configs []Config) (context.Context, string, func(context.Context) string, logrus.Fields, string, string, interface{}, map[string][]byte, error) {
	rpc := &rpcShiroClient{baseConfig: nil, defaultLog: nil, httpClient: http.Client{}}
	ro, err := rpc.applyConfigs(configs...)
	if err != nil {
		return nil, "", nil, nil, "", "", nil, nil, err
	}
	return ro.ctx, ro.id, ro.timestampGenerator, ro.logFields, ro.authToken, ro.creator, ro.params, ro.transient, nil
}

// ProbeForNew returns the option values required for creating a new
// mock implied by an array of options.
func ProbeForNew(configs []Config) (bool, *url.URL, error) {
	rpc := &rpcShiroClient{baseConfig: nil, defaultLog: nil, httpClient: http.Client{}}
	ro, err := rpc.applyConfigs(configs...)
	if err != nil {
		return false, nil, err
	}
	return ro.ccFetchURLDowngrade, ro.ccFetchURLProxy, nil
}

// ShiroClient is an abstraction for a connection to a
// blockchain-based smart contract execution engine. Currently, the
// "phylum" code must be written in a LISP dialect known as Elps.
type ShiroClient interface {
	// Seed re-opens the ShiroClient, specifying the phylum version to
	// target.
	Seed(version string, config ...Config) error

	// ShiroPhylum returns a non-empty string which should act as an
	// indentifier indicating the deployed phylum code being executed by
	// the shiro server.
	ShiroPhylum(config ...Config) (string, error)

	// Init initializes the chaincode given a string containing
	// base64-encoded phylum code.  The phylum code should be deployed
	// with the identifier returned by method ShiroPhylum().
	Init(phylum string, config ...Config) error

	// Call executes method with the given parameters and commits the
	// results.  The method shuold be executed by the phylum code
	// matching the identifier returned by method ShiroPhylum().
	//
	// Caller may specify transient data that is accessible to the
	// chaincode but not comitted on to the blockchain.
	Call(ctx context.Context, method string, config ...Config) (ShiroResponse, error)

	// QueryInfo returns the blockchain height.
	QueryInfo(config ...Config) (uint64, error)

	// QueryBlock returns summary information about the block given by
	// blockNumber.
	QueryBlock(blockNumber uint64, config ...Config) (Block, error)
}

// MockShiroClient is an abstraction for a ShiroClient that is backed
// by an in-process lightweight ledger.
type MockShiroClient interface {
	ShiroClient

	// Close shuts down the mock backing database.
	Close() error

	// Snapshot copies the current state of the mock backend out
	// to the supplied io.Writer.
	Snapshot(w io.Writer) error

	// SetCreatorWithAttributes sets the transaction creator and
	// their attributes.  Any previously set creator attributes
	// are discarded.
	SetCreatorWithAttributes(creator string, attrs map[string]string) error
}

// ShiroResponse is a wrapper for a response from a shiro
// chaincode. Even if the chaincode was invoked successfully, it may
// have signaled an error.
type ShiroResponse interface {
	UnmarshalTo(dst interface{}) error
	ResultJSON() []byte
	TransactionID() string
	Error() Error
}

// Error is a generic application error.
type Error interface {
	// Code returns a numeric code categorizing the error.
	Code() int

	// Message returns a generic error message that corresponds to the
	// error Code.
	Message() string

	// DataJSON returns JSON data returned by the application with the
	// error, if any was provided. The slice returned by DataJSON will
	// either be empty or it will contain valid serialized JSON data.
	DataJSON() []byte
}

// Block is a wrapper for summary information about a block.
type Block interface {
	Hash() string
	Transactions() []Transaction
}

// Transaction is a wrapper for summary information about a transaction.
type Transaction interface {
	ID() string
	Reason() string
	Event() []byte
	ChaincodeID() string
}

// RESPONSE IMPLEMENTATIONS

type successResponse struct {
	result []byte
	txID   string
}

func (s *successResponse) UnmarshalTo(dst interface{}) error {
	message, ok := dst.(proto.Message)
	if ok {
		return jsonpb.Unmarshal(bytes.NewReader(s.result), message)
	}
	return json.Unmarshal(s.result, dst)
}

func (s *successResponse) ResultJSON() []byte {
	out := make([]byte, len(s.result))
	copy(out, s.result)
	return out
}

func (s *successResponse) TransactionID() string {
	return s.txID
}

func (s *successResponse) Error() Error {
	return nil
}

type failureResponse struct {
	code    int
	message string
	data    []byte
}

func (s *failureResponse) UnmarshalTo(dst interface{}) error {
	return errors.New("can't unmarshal the result if the RPC call failed")
}

func (s *failureResponse) ResultJSON() []byte {
	return nil
}

func (s *failureResponse) TransactionID() string {
	return ""
}

func (s *failureResponse) Error() Error {
	return s
}

func (s *failureResponse) Code() int {
	return s.code
}

func (s *failureResponse) Message() string {
	return s.message
}

func (s *failureResponse) DataJSON() []byte {
	out := make([]byte, len(s.data))
	copy(out, s.data)
	return out
}

// BLOCK IMPLEMENTATION

type block struct {
	hash         string
	transactions []Transaction
}

var _ Block = &block{}

// Hash implements Block
func (b *block) Hash() string {
	return b.hash
}

// Transactions implements Block
func (b *block) Transactions() []Transaction {
	out := make([]Transaction, len(b.transactions))
	copy(out, b.transactions)
	return out
}

type transaction struct {
	id     string
	reason string
	event  []byte
	ccID   string
}

var _ Transaction = &transaction{}

// ID implements Transaction
func (t *transaction) ID() string {
	return t.id
}

// Reason implements Transaction
func (t *transaction) Reason() string {
	return t.reason
}

// Event implements Transaction
func (t *transaction) Event() []byte {
	return t.event
}

// ChaincodeID implements Transaction
func (t *transaction) ChaincodeID() string {
	return t.ccID
}

// RPC IMPLEMENTATION - forwards to a JSON-RPC shiro gateway server

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

var _ ShiroClient = (*rpcShiroClient)(nil)

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
}

// Error implements error.
func (e *scError) Error() string {
	return e.message
}

// IsTimeoutError inspects an error returned from shiroclient and returns true
// if it's a timeout.
func IsTimeoutError(err error) bool {
	se, ok := err.(*scError)
	if !ok {
		return false
	}
	return se.code == ErrorCodeShiroClientTimeout
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

// reqres is a round-trip "request/response" helper. Marshals "req",
// logs it at debug level, makes the HTTP request, reads and logs the
// response at debug level, unmarshals, parses into rpcres.
func (c *rpcShiroClient) reqres(req interface{}, opt *requestOptions) (*rpcres, error) {
	outmsg, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if opt.endpoint == "" {
		return nil, errors.New("ShiroClient.reqres expected an endpoint to be set")
	}

	ctx := opt.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", opt.endpoint, bytes.NewReader(outmsg))
	if err != nil {
		return nil, err
	}

	for k, v := range opt.headers {
		httpReq.Header.Set(k, v)
	}
	if opt.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+opt.authToken)
	}

	httpRes, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	defer io.Copy(ioutil.Discard, httpRes.Body)
	defer httpRes.Body.Close()

	msg, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var target *interface{}

	if opt.target == nil {
		var resArb interface{}
		target = &resArb
	} else {
		target = opt.target
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
func (c *rpcShiroClient) applyConfigs(configs ...Config) (*requestOptions, error) {
	uuid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	opt := &requestOptions{
		log:                 c.defaultLog,
		logFields:           make(logrus.Fields),
		headers:             map[string]string{},
		endpoint:            "",
		id:                  uuid.String(),
		params:              nil,
		transient:           map[string][]byte{},
		target:              nil,
		timestampGenerator:  nil,
		mspFilter:           nil,
		minEndorsers:        0,
		creator:             "",
		dependentTxID:       "",
		disableWritePolling: false,
	}

	for _, config := range c.baseConfig {
		config(opt)
	}

	for _, config := range configs {
		config(opt)
	}

	return opt, nil
}

// Seed implements the ShiroClient interface.
func (c *rpcShiroClient) Seed(version string, configs ...Config) error {
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
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
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return "", err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
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
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
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
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return nil, err
	}

	transientJSON := make(map[string]interface{})

	for k, v := range opt.transient {
		transientJSON[k] = hex.EncodeToString(v)
	}

	if opt.timestampGenerator != nil {
		transientJSON["timestamp_override"] = hex.EncodeToString([]byte(opt.timestampGenerator(ctx)))
	}

	params := map[string]interface{}{
		"method":    method,
		"params":    opt.params,
		"transient": transientJSON,
	}
	if opt.dependentTxID != "" {
		params["dependent_txid"] = opt.dependentTxID
	}
	if opt.disableWritePolling {
		params["disable_write_polling"] = opt.disableWritePolling
	}
	params["cc_fetchurl_downgrade"] = opt.ccFetchURLDowngrade
	if opt.ccFetchURLProxy != nil {
		params["cc_fetchurl_proxy"] = opt.ccFetchURLProxy.String()
	} else {
		params["cc_fetchurl_proxy"] = ""
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
		"method":  MethodCall,
		"params":  params,
	}

	if len(opt.mspFilter) > 0 {
		req["params"].(map[string]interface{})["msp_filter"] = opt.mspFilter
	}

	if opt.minEndorsers > 0 {
		req["params"].(map[string]interface{})["min_endorsers"] = opt.minEndorsers
	}

	if opt.creator != "" {
		req["params"].(map[string]interface{})["creator_msp_id"] = opt.creator
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

		return &successResponse{
			result: resultJSON,
			txID:   res.txID,
		}, nil

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

		return &failureResponse{code: int(code), message: message, data: dataJSON}, nil

	default:
		return nil, fmt.Errorf("ShiroClient.Call unexpected error level %d", res.errorLevel)
	}
}

// QueryInfo implements the ShiroClient interface.
func (c *rpcShiroClient) QueryInfo(configs ...Config) (uint64, error) {
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return 0, err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
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
	opt, err := c.applyConfigs(configs...)
	if err != nil {
		return nil, err
	}

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      opt.id,
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
			transactions[i] = &transaction{id: txid, reason: reasonsOut[i], event: eventsOut[i], ccID: ccidsOut[i]}
		}

		return &block{hash: blockHash, transactions: transactions}, nil

	case ErrorLevelShiroClient:
		return nil, res.getShiroClientError()

	default:
		return nil, fmt.Errorf("ShiroClient.QueryBlock unexpected error level %d", res.errorLevel)
	}
}

// NewRPC creates a new RPC ShiroClient with the given set of base
// configs that will be applied to all commands.
func NewRPC(configs ...Config) ShiroClient {
	return &rpcShiroClient{baseConfig: configs, defaultLog: logrus.New(), httpClient: http.Client{}}
}

// MOCK IMPLEMENTATION - forwards to Plugin

type mockShiroClient struct {
	baseConfig  []Config
	defaultLog  *logrus.Logger
	conn        substratecommon.Substrate
	tag         string
	shiroPhylum string
}

var _ MockShiroClient = (*mockShiroClient)(nil)

// applyConfigs applies configs -- baseConfigs supplied in the
// constructor first, followed by configs arguments.
func (c *mockShiroClient) flatten(configs ...Config) (*substratecommon.ConcreteRequestOptions, error) {
	opt := &requestOptions{
		logFields: make(logrus.Fields),
		id:        uuid.NewString(),
		headers:   map[string]string{},
		transient: map[string][]byte{},
		params:    nil,
	}

	for _, config := range c.baseConfig {
		config(opt)
	}

	for _, config := range configs {
		config(opt)
	}

	params, err := json.Marshal(opt.params)
	if err != nil {
		return nil, err
	}

	tsg := (func(ctx context.Context, tg func(context.Context) string) string {
		if tg != nil {
			return tg(ctx)
		}

		return time.Now().UTC().Format(time.RFC3339)
	})

	url := (func(x *url.URL) string {
		out := ""

		if x != nil {
			out = x.String()
		}

		return out
	})

	return &substratecommon.ConcreteRequestOptions{
		Headers:             opt.headers,
		Endpoint:            opt.endpoint,
		ID:                  opt.id,
		AuthToken:           opt.authToken,
		Params:              params,
		Transient:           opt.transient,
		Timestamp:           tsg(opt.ctx, opt.timestampGenerator),
		MSPFilter:           opt.mspFilter,
		MinEndorsers:        opt.minEndorsers,
		Creator:             opt.creator,
		DependentTxID:       opt.dependentTxID,
		DisableWritePolling: opt.disableWritePolling,
		CCFetchURLDowngrade: opt.ccFetchURLDowngrade,
		CCFetchURLProxy:     url(opt.ccFetchURLProxy),
	}, nil
}

// Seed implements the ShiroClient interface.
func (c *mockShiroClient) Seed(version string, configs ...Config) error {
	return fmt.Errorf("Seed(...) is not supported")
}

// ShiroPhylum implements the ShiroClient interface.
func (c *mockShiroClient) ShiroPhylum(configs ...Config) (string, error) {
	return c.shiroPhylum, nil
}

// Init implements the ShiroClient interface.
func (c *mockShiroClient) Init(phylum string, configs ...Config) error {
	cro, err := c.flatten(configs...)
	if err != nil {
		return err
	}
	return c.conn.Init(c.tag, phylum, cro)
}

// Call implements the ShiroClient interface.
func (c *mockShiroClient) Call(ctx context.Context, method string, configs ...Config) (ShiroResponse, error) {
	cro, err := c.flatten(configs...)
	if err != nil {
		return nil, err
	}

	resp, err := c.conn.Call(c.tag, method, cro)
	if err != nil {
		return nil, err
	}

	if resp.HasError {
		return &failureResponse{code: resp.ErrorCode, message: resp.ErrorMessage, data: resp.ErrorJSON}, nil
	}

	return &successResponse{
		result: resp.ResultJSON,
		txID:   resp.TransactionID,
	}, nil
}

// QueryInfo implements the ShiroClient interface.
func (c *mockShiroClient) QueryInfo(configs ...Config) (uint64, error) {
	cro, err := c.flatten(configs...)
	if err != nil {
		return 0, err
	}

	return c.conn.QueryInfo(c.tag, cro)
}

// QueryBlock implements the ShiroClient interface.
func (c *mockShiroClient) QueryBlock(blockNumber uint64, configs ...Config) (Block, error) {
	cro, err := c.flatten(configs...)
	if err != nil {
		return nil, err
	}

	blk, err := c.conn.QueryBlock(c.tag, blockNumber, cro)
	if err != nil {
		return nil, err
	}

	transactionsIn := blk.Transactions

	transactions := make([]Transaction, len(transactionsIn))

	for _, transactionIn := range transactionsIn {
		transactions = append(transactions, &transaction{id: transactionIn.ID, reason: transactionIn.Reason, event: transactionIn.Event})
	}

	return &block{transactions: transactions}, nil
}

// Snapshot copies the current state of the mock backend out to the supplied
// io.Writer.
func (c *mockShiroClient) Snapshot(w io.Writer) error {
	bytes, err := c.conn.SnapshotMock(c.tag)
	if err != nil {
		return err
	}
	_, err = w.Write(bytes)
	return err
}

// SetCreatorWithAttributes sets the transaction creator and their attributes.
// Any previously set creator attributes are discarded.
func (c *mockShiroClient) SetCreatorWithAttributes(creator string, attrs map[string]string) error {
	return c.conn.SetCreatorWithAttributesMock(c.tag, creator, attrs)
}

// Close shuts down the mock backing database
func (c *mockShiroClient) Close() error {
	return c.conn.CloseMock(c.tag)
}

// NewMock creates a new mock ShiroClient with the given set of base
// configs that will be applied to all commands.
func NewMock(conn substratecommon.Substrate, name string, version string, configs ...Config) (MockShiroClient, error) {
	return NewMockFrom(conn, name, version, nil, configs...)
}

// NewMockFrom creates a new mock ShiroClient with the given set of base configs
// that will be applied to all commands.  The state is restored from the
// supplied io.Reader.
func NewMockFrom(conn substratecommon.Substrate, name string, version string, r io.Reader, configs ...Config) (MockShiroClient, error) {
	var err error

	var snapshot []byte

	if r != nil {
		snapshot, err = ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
	}

	var tag string

	tag, err = conn.NewMockFrom(name, version, snapshot)

	return &mockShiroClient{baseConfig: configs, defaultLog: logrus.New(), conn: conn, tag: tag, shiroPhylum: name}, nil
}

type syncClient struct {
	mutex      *sync.Mutex
	underlying MockShiroClient
}

var _ MockShiroClient = (*syncClient)(nil)

// Seed implements the ShiroClient interface.
func (c *syncClient) Seed(version string, configs ...Config) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.Seed(version, configs...)
}

// ShiroPhylum implements the ShiroClient interface.
func (c *syncClient) ShiroPhylum(configs ...Config) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.ShiroPhylum(configs...)
}

// Init implements the ShiroClient interface.
func (c *syncClient) Init(phylum string, configs ...Config) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.Init(phylum, configs...)
}

// Call implements the ShiroClient interface.
func (c *syncClient) Call(ctx context.Context, method string, configs ...Config) (ShiroResponse, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.Call(ctx, method, configs...)
}

// QueryInfo implements the ShiroClient interface.
func (c *syncClient) QueryInfo(configs ...Config) (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.QueryInfo(configs...)
}

// QueryBlock implements the ShiroClient interface.
func (c *syncClient) QueryBlock(blockNumber uint64, configs ...Config) (Block, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.QueryBlock(blockNumber, configs...)
}

// Close implements the MockShiroClient interface.
func (c *syncClient) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.Close()
}

// Snapshot implements the MockShiroClient interface.
func (c *syncClient) Snapshot(w io.Writer) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.Snapshot(w)
}

// Snapshot implements the MockShiroClient interface.
func (c *syncClient) SetCreatorWithAttributes(creator string, attrs map[string]string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.underlying.SetCreatorWithAttributes(creator, attrs)
}

// NewSync returns a ShiroClient that will be synchronized to be
// usable from more than one goroutine when the underlying
// implementation is not thread-safe.
func NewSync(shiroclient MockShiroClient) MockShiroClient {
	return &syncClient{mutex: &sync.Mutex{}, underlying: shiroclient}
}

// EncodePhylumBytes takes decoded phylum (lisp code) and encodes it
// for use with the Init() method.
func EncodePhylumBytes(decoded []byte) string {
	return base64.StdEncoding.EncodeToString(decoded)
}
