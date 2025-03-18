package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/luthersystems/shiroclient-sdk-go/internal/mockint"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
)

var _ types.ShiroClient = (*mockShiroClient)(nil)

var _ MockShiroClient = (*mockShiroClient)(nil)

type MockShiroClient interface {
	types.ShiroClient
	Close() error
	Snapshot(w io.Writer) error
	SetCreatorWithAttributes(creator string, attrs map[string]string) error
}

type mockShiroClient struct {
	baseConfig  []types.Config
	conn        *plugin.SubstrateConnection
	tag         string
	shiroPhylum string
}

func (c *mockShiroClient) flatten(ctx context.Context, configs ...types.Config) (*plugin.ConcreteRequestOptions, error) {
	opt := types.ApplyConfigs(nil, append(c.baseConfig, configs...)...)

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
	return c.conn.GetSubstrate().Init(c.tag, phylum, cro)
}

// Call implements the ShiroClient interface.
func (c *mockShiroClient) Call(ctx context.Context, method string, configs ...types.Config) (types.ShiroResponse, error) {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return nil, err
	}

	resp, err := c.conn.GetSubstrate().Call(c.tag, method, cro)
	if err != nil {
		return nil, err
	}

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

	return c.conn.GetSubstrate().QueryInfo(c.tag, cro)
}

// QueryBlock implements the ShiroClient interface.
func (c *mockShiroClient) QueryBlock(ctx context.Context, blockNumber uint64, configs ...types.Config) (types.Block, error) {
	cro, err := c.flatten(ctx, configs...)
	if err != nil {
		return nil, err
	}

	blk, err := c.conn.GetSubstrate().QueryBlock(c.tag, blockNumber, cro)
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
	bytes, err := c.conn.GetSubstrate().SnapshotMock(c.tag)
	if err != nil {
		return err
	}
	_, err = w.Write(bytes)
	return err
}

// SetCreatorWithAttributes sets the transaction creator and their attributes.
// Any previously set creator attributes are discarded.
func (c *mockShiroClient) SetCreatorWithAttributes(creator string, attrs map[string]string) error {
	return c.conn.GetSubstrate().SetCreatorWithAttributesMock(c.tag, creator, attrs)
}

// Close shuts down the mock backing database
func (c *mockShiroClient) Close() error {
	errMock := c.conn.GetSubstrate().CloseMock(c.tag)
	errPlugin := c.conn.Close()
	if errMock != nil {
		return fmt.Errorf("failed to close mock client: %w", errMock)
	}
	if errPlugin != nil {
		return fmt.Errorf("failed to close plugin: %w", errPlugin)
	}
	return nil
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
		LogWriter: os.Stdout,
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
		plugin.ConnectWithAttachStdamp(config.LogWriter),
	}
	conn, err := plugin.NewSubstrateConnection(pluginOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to plugin: %w", err)
	}
	var snapshot []byte
	if config.SnapshotReader != nil {
		snapshot, err = io.ReadAll(config.SnapshotReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read snapshot: %w", err)
		}
	}
	var tag string
	tag, err = conn.GetSubstrate().NewMockFrom(mockint.PhylumName, mockint.PhylumVersion, snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to create mock client: %w", err)
	}
	return &mockShiroClient{
		baseConfig:  clientConfigs,
		conn:        conn,
		tag:         tag,
		shiroPhylum: mockint.PhylumName,
	}, nil
}
