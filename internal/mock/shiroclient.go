package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/luthersystems/shiroclient-sdk-go/internal/mockint"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
	"github.com/luthersystems/svc/txctx"
	"go.opentelemetry.io/otel/propagation"
)

// tracePropagator carries the W3C TraceContext across the goplugin RPC
// boundary into substratehcp, where the original ctx is otherwise lost.
// Mirrors the propagator used in internal/rpc/shiroclient.go.
var tracePropagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})

// traceCarrier adapts ConcreteRequestOptions.Transient (map[string][]byte) to
// propagation.TextMapCarrier so trace headers can ride alongside other
// transient data without a string/[]byte copy.
type traceCarrier map[string][]byte

func (c traceCarrier) Get(key string) string {
	if v, ok := c[key]; ok {
		return string(v)
	}
	return ""
}

func (c traceCarrier) Set(key, value string) { c[key] = []byte(value) }

func (c traceCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

var _ types.ShiroClient = (*mockShiroClient)(nil)

var _ MockShiroClient = (*mockShiroClient)(nil)

type MockShiroClient interface {
	types.ShiroClient
	Close() error
	Snapshot(w io.Writer) error
	SetCreatorWithAttributes(creator string, attrs map[string]string) error
}

type mockShiroClient struct {
	baseConfig []types.Config
	conn       *plugin.SubstrateConnection
	// substrate is conn's Substrate (a fake in tests).
	substrate plugin.Substrate
	// release gives back the plugin connection: it kills a private
	// process, or drops a reference to a shared one.
	release     func() error
	tag         string
	shiroPhylum string
	closeOnce   sync.Once
	closeErr    error
}

// flatten is the single choke point for every RPC-bound method (Init, Call,
// QueryInfo, QueryBlock). When adding a new entry point, route it through
// flatten with the caller's ctx so trace propagation, timestamps, and config
// merge order stay consistent across the goplugin boundary.
func (c *mockShiroClient) flatten(ctx context.Context, configs ...types.Config) (*plugin.ConcreteRequestOptions, error) {
	return c.concrete(ctx, c.options(ctx, configs...))
}

// options applies the base configs, then configs, and injects the trace
// context into the transient data.
func (c *mockShiroClient) options(ctx context.Context, configs ...types.Config) *types.RequestOptions {
	opt := types.ApplyConfigs(nil, append(c.baseConfig, configs...)...)
	tracePropagator.Inject(ctx, traceCarrier(opt.Transient))
	return opt
}

// concrete flattens opt to the plugin's pure-data options.
func (c *mockShiroClient) concrete(ctx context.Context, opt *types.RequestOptions) (*plugin.ConcreteRequestOptions, error) {
	params, err := json.Marshal(opt.Params)
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

	return &plugin.ConcreteRequestOptions{
		Headers:             opt.Headers,
		Endpoint:            opt.Endpoint,
		ID:                  opt.ID,
		AuthToken:           opt.AuthToken,
		Params:              params,
		Transient:           opt.Transient,
		Timestamp:           tsg(ctx, opt.TimestampGenerator),
		MSPFilter:           opt.MspFilter,
		MinEndorsers:        opt.MinEndorsers,
		Creator:             opt.Creator,
		DependentTxID:       opt.DependentTxID,
		DependentBlock:      opt.DependentBlock,
		DisableWritePolling: opt.DisableWritePolling,
		PhylumVersion:       opt.PhylumVersion,
		NewPhylumVersion:    opt.NewPhylumVersion,
		CCFetchURLDowngrade: opt.CcFetchURLDowngrade,
		CCFetchURLProxy:     url(opt.CcFetchURLProxy),
		DebugPrint:          opt.DebugPrint,
	}, nil
}

// Seed implements the ShiroClient interface.
func (c *mockShiroClient) Seed(_ context.Context, version string, configs ...types.Config) error {
	return fmt.Errorf("Seed(...) is not supported")
}

// ShiroPhylum implements the ShiroClient interface.
func (c *mockShiroClient) ShiroPhylum(_ context.Context, configs ...types.Config) (string, error) {
	return c.shiroPhylum, nil
}

// Init implements the ShiroClient interface.
func (c *mockShiroClient) Init(ctx context.Context, phylum string, configs ...types.Config) error {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return err
	}
	return c.substrate.Init(c.tag, phylum, cro)
}

// Call implements the ShiroClient interface.
func (c *mockShiroClient) Call(ctx context.Context, method string, configs ...types.Config) (types.ShiroResponse, error) {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return nil, err
	}
	if err := types.CheckTransientKeys(cro.Transient); err != nil {
		return nil, fmt.Errorf("ShiroClient.Call: %w", err)
	}

	resp, err := c.substrate.Call(c.tag, method, cro)
	if err != nil {
		return nil, err
	}

	txctx.SetTransactionDetails(ctx, txctx.TransactionDetails{TransactionID: resp.TransactionID})

	if resp.HasError {
		return types.NewFailureResponse(resp.ErrorCode, resp.ErrorMessage, resp.ErrorJSON), nil
	}

	return types.NewSuccessResponse(resp.ResultJSON, resp.TransactionID, 0, 0), nil
}

// QueryInfo implements the ShiroClient interface.
func (c *mockShiroClient) QueryInfo(ctx context.Context, configs ...types.Config) (uint64, error) {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return 0, err
	}

	return c.substrate.QueryInfo(c.tag, cro)
}

// QueryBlock implements the ShiroClient interface.
func (c *mockShiroClient) QueryBlock(ctx context.Context, blockNumber uint64, configs ...types.Config) (types.Block, error) {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return nil, err
	}

	blk, err := c.substrate.QueryBlock(c.tag, blockNumber, cro)
	if err != nil {
		return nil, err
	}

	transactionsIn := blk.Transactions

	transactions := make([]types.Transaction, len(transactionsIn))

	for i, transactionIn := range transactionsIn {
		transactions[i] = types.NewTransaction(transactionIn.ID, transactionIn.Reason, transactionIn.Event, transactionIn.ChaincodeID)
	}

	return types.NewBlock(blk.Hash, transactions), nil
}

// Snapshot copies the current state of the mock backend out to the supplied
// io.Writer.
func (c *mockShiroClient) Snapshot(w io.Writer) error {
	bytes, err := c.substrate.SnapshotMock(c.tag)
	if err != nil {
		return err
	}
	_, err = w.Write(bytes)
	return err
}

// SetCreatorWithAttributes sets the transaction creator and their attributes.
// Any previously set creator attributes are discarded.
func (c *mockShiroClient) SetCreatorWithAttributes(creator string, attrs map[string]string) error {
	return c.substrate.SetCreatorWithAttributesMock(c.tag, creator, attrs)
}

// Close shuts down the mock backing database and releases the plugin
// connection: a private plugin process is stopped, a shared one is stopped
// only once no other mock uses it. Calls after the first return the first
// result.
func (c *mockShiroClient) Close() error {
	c.closeOnce.Do(func() {
		errMock := c.substrate.CloseMock(c.tag)
		errPlugin := c.release()
		if errMock != nil {
			c.closeErr = fmt.Errorf("failed to close mock client: %w", errMock)
		} else if errPlugin != nil {
			c.closeErr = fmt.Errorf("failed to close plugin: %w", errPlugin)
		}
	})
	return c.closeErr
}

func hcpLogLevel(mockLevel mockint.LogLevel) hclog.Level {
	switch mockLevel {
	case mock.Debug:
		return hclog.Debug
	case mock.Info:
		return hclog.Info
	case mock.Warn:
		return hclog.Warn
	case mock.Error:
		return hclog.Error
	default:
		return hclog.DefaultLevel
	}
}

func NewMock(clientConfigs []types.Config, opts ...mock.Option) (MockShiroClient, error) {
	config := &mockint.Config{
		LogWriter:         os.Stdout,
		SharedIdleTimeout: mockint.DefaultSharedIdleTimeout,
	}
	for _, opt := range opts {
		opt(config)
	}
	if config.PluginPath == "" {
		config.PluginPath = os.Getenv(mockint.DefaultPluginEnv)
		if config.PluginPath == "" {
			return nil, fmt.Errorf("%s not found in environment", mockint.DefaultPluginEnv)
		}
	}
	pluginOpts := []plugin.ConnectOption{
		plugin.ConnectWithCommand(config.PluginPath),
		plugin.ConnectWithLogLevel(hcpLogLevel(config.LogLevel)),
		// LogWriter drives BOTH plugin log streams. It fed only the
		// subprocess's stdio before, while go-plugin's own host-side client
		// logger stayed hardcoded to os.Stdout -- so mock.WithLogWriter did
		// not do what it documents ("sets the plugin's log destination"), and
		// a caller could not silence the plugin at all.
		plugin.ConnectWithAttachStdamp(config.LogWriter),
		plugin.ConnectWithLogOutput(config.LogWriter),
	}
	var snapshot []byte
	if config.SnapshotReader != nil {
		var err error
		snapshot, err = io.ReadAll(config.SnapshotReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read snapshot: %w", err)
		}
	}
	connect := func() (*plugin.SubstrateConnection, error) {
		conn, err := plugin.NewSubstrateConnection(pluginOpts...)
		if err != nil {
			return nil, fmt.Errorf("unable to connect to plugin: %w", err)
		}
		return conn, nil
	}
	var (
		conn    *plugin.SubstrateConnection
		release func() error
	)
	if config.SharedPlugin && canShare(config.LogWriter) {
		key := sharedKey{writer: config.LogWriter, path: config.PluginPath, level: hcpLogLevel(config.LogLevel)}
		sc, err := acquireShared(key, config.SharedIdleTimeout, connect)
		if err != nil {
			return nil, err
		}
		conn = sc.conn
		release = func() error { return releaseShared(sc) }
	} else {
		var err error
		conn, err = connect()
		if err != nil {
			return nil, err
		}
		release = conn.Close
	}
	tag, err := conn.GetSubstrate().NewMockFrom(mockint.PhylumName, mockint.PhylumVersion, snapshot, plugin.MockOptions{
		PreheatTimeout: config.PreheatTimeout,
	})
	if err != nil {
		_ = release()
		return nil, fmt.Errorf("failed to create mock client: %w", err)
	}
	return &mockShiroClient{
		baseConfig:  clientConfigs,
		conn:        conn,
		substrate:   conn.GetSubstrate(),
		release:     release,
		tag:         tag,
		shiroPhylum: mockint.PhylumName,
	}, nil
}

var (
	_ types.CallBatcher  = (*mockShiroClient)(nil)
	_ types.QueryBatcher = (*mockShiroClient)(nil)
)

// CallBatch implements types.CallBatcher through the plugin's
// BatchSubstrate.  Batches need a substratehcp release that implements
// plugin.BatchSubstrate; an older plugin yields
// types.ErrCallBatchNotSupported.  All-or-nothing semantics are never faked
// by issuing the requests as separate Calls.
func (c *mockShiroClient) CallBatch(ctx context.Context, requests []types.CallBatchRequest, configs ...types.Config) (*types.CallBatchResponse, error) {
	return c.runBatch(ctx, false, requests, configs)
}

// QueryBatch implements types.QueryBatcher through the plugin's
// BatchSubstrate, like CallBatch; an older plugin yields
// types.ErrQueryBatchNotSupported.
func (c *mockShiroClient) QueryBatch(ctx context.Context, requests []types.CallBatchRequest, configs ...types.Config) (*types.CallBatchResponse, error) {
	return c.runBatch(ctx, true, requests, configs)
}

func (c *mockShiroClient) runBatch(ctx context.Context, query bool, requests []types.CallBatchRequest, configs []types.Config) (*types.CallBatchResponse, error) {
	name, unsupported := "ShiroClient.CallBatch", types.ErrCallBatchNotSupported
	if query {
		name, unsupported = "ShiroClient.QueryBatch", types.ErrQueryBatchNotSupported
	}
	opt := c.options(ctx, configs...)
	prepared, err := types.PrepareBatch(name, requests, opt)
	if err != nil {
		return nil, err
	}
	// Each request carries its own Params; the batch's are ignored.
	opt.Params = nil
	cro, err := c.concrete(ctx, opt)
	if err != nil {
		return nil, err
	}
	args := make([]plugin.BatchRequestArgs, len(prepared))
	for i, r := range prepared {
		args[i] = plugin.BatchRequestArgs{Method: r.Method, Params: []byte(r.Params), Transient: r.Transient}
		if r.ID != nil {
			if args[i].ID, err = json.Marshal(r.ID); err != nil {
				return nil, fmt.Errorf("%s: request %d: id: %w", name, i, err)
			}
		}
	}

	bs, ok := c.substrate.(plugin.BatchSubstrate)
	if !ok {
		return nil, fmt.Errorf("%w: the mock substrate plugin does not support batches", unsupported)
	}
	run := bs.CallBatch
	if query {
		run = bs.QueryBatch
	}
	res, err := run(c.tag, args, cro)
	if err != nil {
		if errors.Is(err, plugin.ErrBatchNotSupported) {
			return nil, fmt.Errorf("%w: the mock substrate plugin needs a substratehcp release that implements plugin.BatchSubstrate: %v", unsupported, err)
		}
		return nil, err
	}
	br, failed, err := batchResponse(name, res, len(requests))
	if err != nil {
		return nil, err
	}
	switch {
	case query && br.Committed:
		return nil, fmt.Errorf("%s: plugin reported the batch committed; a QueryBatch never commits", name)
	case br.Committed && failed >= 0:
		return nil, fmt.Errorf("%s: plugin reported the batch committed (txid=%s) with a failed request", name, br.TxID)
	}
	if br.Committed {
		txctx.SetTransactionDetails(ctx, txctx.TransactionDetails{TransactionID: br.TxID})
	}
	if opt.ResponseReceiver != nil {
		for _, r := range br.Responses {
			opt.ResponseReceiver(r)
		}
	}
	if failed >= 0 {
		return br, &types.CallBatchError{Index: failed, ID: br.IDs[failed], Err: br.Responses[failed].Error()}
	}
	return br, nil
}

// batchResponse converts the plugin's answer, returning the failed request's
// index or -1.
func batchResponse(name string, res *plugin.BatchResponse, n int) (*types.CallBatchResponse, int, error) {
	if res == nil || len(res.Responses) != n {
		return nil, 0, fmt.Errorf("%s: plugin returned the wrong number of responses for %d requests", name, n)
	}
	br := &types.CallBatchResponse{
		Responses: make([]types.ShiroResponse, n),
		IDs:       make([]interface{}, n),
		Committed: res.Committed,
	}
	if res.Committed {
		br.TxID = res.TransactionID
	}
	for i, r := range res.Responses {
		if r == nil {
			return nil, 0, fmt.Errorf("%s: plugin returned no response for request %d", name, i)
		}
		if i < len(res.IDs) && len(res.IDs[i]) > 0 {
			if err := json.Unmarshal(res.IDs[i], &br.IDs[i]); err != nil {
				return nil, 0, fmt.Errorf("%s: id of response %d: %w", name, i, err)
			}
		}
		if r.HasError {
			br.Responses[i] = types.NewFailureResponse(r.ErrorCode, r.ErrorMessage, r.ErrorJSON)
		} else {
			br.Responses[i] = types.NewSuccessResponse(r.ResultJSON, br.TxID, 0, 0)
		}
	}
	failed := br.FailedIndex()
	if res.FailedIndex >= 0 && res.FailedIndex < n && br.Responses[res.FailedIndex].Error() != nil {
		failed = res.FailedIndex
	}
	return br, failed, nil
}
