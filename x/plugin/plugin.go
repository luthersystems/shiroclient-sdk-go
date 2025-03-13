// Package plugin includes helpers for the substrate plugin implementation
// to extract configuration arguments.
// WARNING: This is unstable and really should only be used by the underlying
// substrate implementation. It will be removed in later versions.
package plugin

import (
	"github.com/sirupsen/logrus"
	"io"
	"log"
	"net/rpc"
	"os"
	"os/exec"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
)

// ConcreteRequestOptions is a variant of RequestOptions that is
// "flattened" to pure data.
type ConcreteRequestOptions struct {
	Headers             map[string]string
	Endpoint            string
	ID                  string
	AuthToken           string
	Params              []byte
	Transient           map[string][]byte
	Timestamp           string
	MSPFilter           []string
	MinEndorsers        int
	Creator             string
	DependentTxID       string
	DisableWritePolling bool
	CCFetchURLDowngrade bool
	CCFetchURLProxy     string
	DependentBlock      string
	PhylumVersion       string
	NewPhylumVersion    string
	DebugPrint          bool
}

// Error represents a possible error.
type Error struct {
	Diagnostic string
}

// Error returns the underlying details.
func (e Error) Error() string {
	return e.Diagnostic
}

// Response represents a shiroclient response.
type Response struct {
	ResultJSON    []byte
	HasError      bool
	ErrorCode     int
	ErrorMessage  string
	ErrorJSON     []byte
	TransactionID string
}

// UnmarshalTo unmarshals the response's result to dst.
func (s *Response) UnmarshalTo(dst interface{}) error {
	return types.UnmarshalProto(s.ResultJSON, dst)
}

// Transaction represents summary information about a transaction.
type Transaction struct {
	ID          string
	Reason      string
	Event       []byte
	ChaincodeID string
}

// Block represents summary information about a block.
type Block struct {
	Hash         string
	Transactions []*Transaction
}

// Substrate is the interface that we're exposing as a plugin.
type Substrate interface {
	HealthCheck(int) (int, error)

	NewMockFrom(string, string, []byte) (string, error)
	SetCreatorWithAttributesMock(string, string, map[string]string) error
	SnapshotMock(string) ([]byte, error)
	CloseMock(string) error

	Init(string, string, *ConcreteRequestOptions) error
	Call(string, string, *ConcreteRequestOptions) (*Response, error)
	QueryInfo(string, *ConcreteRequestOptions) (uint64, error)
	QueryBlock(string, uint64, *ConcreteRequestOptions) (*Block, error)
}

// ArgsHealthCheck encodes the arguments to HealthCheck
type ArgsHealthCheck struct {
	Nat int
}

// RespHealthCheck encodes the response from HealthCheck
type RespHealthCheck struct {
	Suc int
}

// ArgsNewMockFrom encodes the arguments to NewMockFrom
type ArgsNewMockFrom struct {
	Name     string
	Version  string
	Snapshot []byte
}

// RespNewMockFrom encodes the response from NewMockFrom
type RespNewMockFrom struct {
	Tag string
	Err *Error
}

// ArgsSetCreatorWithAttributesMock encodes the arguments to SetCreatorWithAttributesMock
type ArgsSetCreatorWithAttributesMock struct {
	Tag     string
	Creator string
	Attrs   map[string]string
}

// RespSetCreatorWithAttributesMock encodes the response from SetCreatorWithAttributesMock
type RespSetCreatorWithAttributesMock struct {
	Err *Error
}

// ArgsSnapshotMock encodes the arguments to SnapshotMock
type ArgsSnapshotMock struct {
	Tag string
}

// RespSnapshotMock encodes the response from SnapshotMock
type RespSnapshotMock struct {
	Snapshot []byte
	Err      *Error
}

// ArgsCloseMock encodes the arguments to CloseMock
type ArgsCloseMock struct {
	Tag string
}

// RespCloseMock encodes the response from CloseMock
type RespCloseMock struct {
	Err *Error
}

// ArgsInit encodes the arguments to Init
type ArgsInit struct {
	Tag     string
	Phylum  string
	Options *ConcreteRequestOptions
}

// RespInit encodes the response from Init
type RespInit struct {
	Err *Error
}

// ArgsCall encodes the arguments to Call
type ArgsCall struct {
	Tag     string
	Command string
	Options *ConcreteRequestOptions
}

// RespCall encodes the response from Call
type RespCall struct {
	Response *Response
	Err      *Error
}

// ArgsQueryInfo encodes the arguments to QueryInfo
type ArgsQueryInfo struct {
	Tag     string
	Options *ConcreteRequestOptions
}

// RespQueryInfo encodes the response from QueryInfo
type RespQueryInfo struct {
	Height uint64
	Err    *Error
}

// ArgsQueryBlock encodes the arguments to QueryBlock
type ArgsQueryBlock struct {
	Tag     string
	Height  uint64
	Options *ConcreteRequestOptions
}

// RespQueryBlock encodes the response from QueryBlock
type RespQueryBlock struct {
	Block *Block
	Err   *Error
}

// PluginRPC is an implementation that talks over RPC
type PluginRPC struct{ client *rpc.Client }

// HealthCheck forwards the call
func (g *PluginRPC) HealthCheck(nat int) (int, error) {
	var resp RespHealthCheck
	err := g.client.Call("Plugin.HealthCheck", &ArgsHealthCheck{Nat: nat}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Suc, nil
}

// NewMockFrom forwards the call
func (g *PluginRPC) NewMockFrom(name string, version string, snapshot []byte) (string, error) {
	var resp RespNewMockFrom
	err := g.client.Call("Plugin.NewMockFrom", &ArgsNewMockFrom{Name: name, Version: version, Snapshot: snapshot}, &resp)
	if err != nil {
		return "", err
	}
	if resp.Err != nil {
		return "", resp.Err
	}
	return resp.Tag, nil
}

// SetCreatorWithAttributesMock forwards the call
func (g *PluginRPC) SetCreatorWithAttributesMock(tag string, creator string, attrs map[string]string) error {
	var resp RespSetCreatorWithAttributesMock
	err := g.client.Call("Plugin.SetCreatorWithAttributesMock", &ArgsSetCreatorWithAttributesMock{Tag: tag, Creator: creator, Attrs: attrs}, &resp)
	if err != nil {
		return err
	}
	if resp.Err != nil {
		return resp.Err
	}
	return nil
}

// SnapshotMock forwards the call
func (g *PluginRPC) SnapshotMock(tag string) ([]byte, error) {
	var resp RespSnapshotMock
	err := g.client.Call("Plugin.SnapshotMock", &ArgsSnapshotMock{Tag: tag}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Err != nil {
		return nil, resp.Err
	}
	return resp.Snapshot, nil
}

// CloseMock forwards the call
func (g *PluginRPC) CloseMock(tag string) error {
	var resp RespCloseMock
	err := g.client.Call("Plugin.CloseMock", &ArgsCloseMock{Tag: tag}, &resp)
	if err != nil {
		return err
	}
	if resp.Err != nil {
		return resp.Err
	}
	return nil
}

// Init forwards the call
func (g *PluginRPC) Init(tag string, phylum string, options *ConcreteRequestOptions) error {
	var resp RespInit
	err := g.client.Call("Plugin.Init", &ArgsInit{Tag: tag, Phylum: phylum, Options: options}, &resp)
	if err != nil {
		return err
	}
	if resp.Err != nil {
		return resp.Err
	}
	return nil
}

// Call forwards the call
func (g *PluginRPC) Call(tag string, command string, options *ConcreteRequestOptions) (*Response, error) {

	if options.DebugPrint {
		logrus.WithFields(logrus.Fields{
			"tag":     tag,
			"command": command,
		}).Debug("UNSAFE: plugin request")
	}

	var resp RespCall
	err := g.client.Call("Plugin.Call", &ArgsCall{Tag: tag, Command: command, Options: options}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Err != nil {
		if options.DebugPrint {
			logrus.WithFields(logrus.Fields{
				"resp.Err": resp.Err.Error(),
			}).Debug("UNSAFE: plugin response error")
		}
		return nil, resp.Err
	}

	if options.DebugPrint {
		logrus.WithFields(logrus.Fields{
			"resp.Response.ResultJSON": string(resp.Response.ResultJSON),
		}).Debug("UNSAFE: plugin response success")
	}

	return resp.Response, nil
}

// QueryInfo forwards the call
func (g *PluginRPC) QueryInfo(tag string, options *ConcreteRequestOptions) (uint64, error) {
	var resp RespQueryInfo
	err := g.client.Call("Plugin.QueryInfo", &ArgsQueryInfo{Tag: tag, Options: options}, &resp)
	if err != nil {
		return 0, err
	}
	if resp.Err != nil {
		return 0, resp.Err
	}
	return resp.Height, nil
}

// QueryBlock forwards the call
func (g *PluginRPC) QueryBlock(tag string, height uint64, options *ConcreteRequestOptions) (*Block, error) {
	var resp RespQueryBlock
	err := g.client.Call("Plugin.QueryBlock", &ArgsQueryBlock{Tag: tag, Height: height, Options: options}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Err != nil {
		return nil, resp.Err
	}
	return resp.Block, nil
}

// PluginRPCServer is the RPC server that PluginRPC talks to,
// conforming to the requirements of net/rpc
type PluginRPCServer struct {
	// This is the real implementation
	Impl Substrate
}

func (s *PluginRPCServer) newError(err error) *Error {
	return &Error{Diagnostic: err.Error()}
}

// HealthCheck forwards the call
func (s *PluginRPCServer) HealthCheck(args *ArgsHealthCheck, resp *RespHealthCheck) error {
	val, err := s.Impl.HealthCheck(args.Nat)
	if err != nil {
		val = -1
	}
	resp.Suc = val
	return nil
}

// NewMockFrom forwards the call
func (s *PluginRPCServer) NewMockFrom(args *ArgsNewMockFrom, resp *RespNewMockFrom) error {
	tag, err := s.Impl.NewMockFrom(args.Name, args.Version, args.Snapshot)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	resp.Tag = tag
	return nil
}

// SetCreatorWithAttributesMock forwards the call
func (s *PluginRPCServer) SetCreatorWithAttributesMock(args *ArgsSetCreatorWithAttributesMock, resp *RespSetCreatorWithAttributesMock) error {
	err := s.Impl.SetCreatorWithAttributesMock(args.Tag, args.Creator, args.Attrs)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	return nil
}

// SnapshotMock forwards the call
func (s *PluginRPCServer) SnapshotMock(args *ArgsSnapshotMock, resp *RespSnapshotMock) error {
	dat, err := s.Impl.SnapshotMock(args.Tag)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	resp.Snapshot = dat
	return nil
}

// CloseMock forwards the call
func (s *PluginRPCServer) CloseMock(args *ArgsCloseMock, resp *RespCloseMock) error {
	err := s.Impl.CloseMock(args.Tag)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	return nil
}

// Init forwards the call
func (s *PluginRPCServer) Init(args *ArgsInit, resp *RespInit) error {
	err := s.Impl.Init(args.Tag, args.Phylum, args.Options)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	return nil
}

// Call forwards the call
func (s *PluginRPCServer) Call(args *ArgsCall, resp *RespCall) error {
	res, err := s.Impl.Call(args.Tag, args.Command, args.Options)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	resp.Response = res
	return nil
}

// QueryInfo forwards the call
func (s *PluginRPCServer) QueryInfo(args *ArgsQueryInfo, resp *RespQueryInfo) error {
	height, err := s.Impl.QueryInfo(args.Tag, args.Options)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	resp.Height = height
	return nil
}

// QueryBlock forwards the call
func (s *PluginRPCServer) QueryBlock(args *ArgsQueryBlock, resp *RespQueryBlock) error {
	block, err := s.Impl.QueryBlock(args.Tag, args.Height, args.Options)
	if err != nil {
		resp.Err = s.newError(err)
		return nil
	}
	resp.Block = block
	return nil
}

// Plugin is the implementation of plugin.Plugin so we can
// serve/consume this.
//
// Ignore MuxBroker. That is used to create more multiplexed streams on our
// plugin connection and is a more advanced use case.
type Plugin struct {
	// Impl Injection
	Impl Substrate
}

// Server returns an RPC server for this plugin type. We construct a
// PluginRPCServer for this.
func (p *Plugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &PluginRPCServer{Impl: p.Impl}, nil
}

// Client returns an implementation of our interface that communicates
// over an RPC client. We return PluginRPC for this.
func (Plugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &PluginRPC{client: c}, nil
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "SUBSTRATEHCP1",
	MagicCookieValue: "substratehcp1",
}

// pluginMap is the map of plugins we can dispense.
var pluginMap = map[string]plugin.Plugin{
	"substrate": &Plugin{},
}

type connectOption struct {
	level        hclog.Level
	command      string
	attachStdamp io.Writer
}

// ConnectOption represents the type of a builder action for connectOption
type ConnectOption func(co *connectOption) error

// ConnectWithLogLevel specifies the log level to use (the default is Debug)
func ConnectWithLogLevel(level hclog.Level) func(co *connectOption) error {
	return (func(co *connectOption) error {
		co.level = level
		return nil
	})
}

// ConnectWithCommand specifies the path to the plugin (the default is "")
func ConnectWithCommand(command string) func(co *connectOption) error {
	return (func(co *connectOption) error {
		co.command = command
		return nil
	})
}

// ConnectWithAttachStdamp specifies an io.Writer to receive stdio output from the plugin
func ConnectWithAttachStdamp(attachStdamp io.Writer) func(co *connectOption) error {
	return (func(co *connectOption) error {
		co.attachStdamp = attachStdamp
		return nil
	})
}

// SubstrateConnection interacts with the underlying plugin.
type SubstrateConnection struct {
	client    *plugin.Client
	substrate Substrate
}

// NewSubstrateConnection connects to a plugin in the background.
func NewSubstrateConnection(opts ...ConnectOption) (*SubstrateConnection, error) {
	co := &connectOption{level: hclog.Debug, attachStdamp: nil}

	for _, opt := range opts {
		if err := opt(co); err != nil {
			panic(err)
		}
	}

	// Create an hclog.Logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  co.level,
	})

	cmd := exec.Command(co.command) // #nosec G204

	// We're a host! Start by launching the plugin process.
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		Cmd:             cmd,
		Logger:          logger,
		Stderr:          co.attachStdamp,
		SyncStdout:      co.attachStdamp,
		SyncStderr:      co.attachStdamp,
	})

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Fatal(err)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("substrate")
	if err != nil {
		log.Fatal(err)
	}

	// This feels like a normal interface implementation but is in
	// fact over an RPC connection.
	substrate := raw.(Substrate)

	return &SubstrateConnection{client: client, substrate: substrate}, nil
}

// GetSubstrate returns the Substrate interface associated with a
// connection.
func (s *SubstrateConnection) GetSubstrate() Substrate {
	return s.substrate
}

// Close closes a connection.
func (s *SubstrateConnection) Close() error {
	s.client.Kill()
	return nil
}

// Connect connects to a plugin synchronously; all operations on the
// Substrate interface must be performed from within the passed
// closure.
func Connect(user func(Substrate) error, opts ...ConnectOption) error {
	conn, err := NewSubstrateConnection(opts...)
	if err != nil {
		return err
	}

	err = user(conn.GetSubstrate())
	if err != nil {
		return err
	}

	return conn.Close()
}

// NewSuccessResponse is used by the plugin to return success ShiroResponse.
func NewSuccessResponse(result []byte, txID string) types.ShiroResponse {
	return types.NewSuccessResponse(result, txID, 0, 0)
}

// NewFailureResponse is used by the plugin to return failure ShiroResponse.
func NewFailureResponse(code int, message string, data []byte) types.ShiroResponse {
	return types.NewFailureResponse(code, message, data)
}

// newShiroClientTransaction is used by the plugin to return transaction details.
func newShiroClientTransaction(tx *Transaction) types.Transaction {
	return types.NewTransaction(tx.ID, tx.Reason, tx.Event, tx.ChaincodeID)
}

// NewShiroClientBlock is used by the plugin to return transaction details.
func NewShiroClientBlock(blk *Block) types.Block {
	txs := make([]types.Transaction, len(blk.Transactions))
	for i, tx := range blk.Transactions {
		txs[i] = newShiroClientTransaction(tx)
	}
	return types.NewBlock(blk.Hash, txs)
}

// WithNewPhylumVersion allows set a new phylum version on install.
// IMPORTANT: this will probably be deleted in a subsequent version.
func WithNewPhylumVersion(phylumVersion string) types.Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.NewPhylumVersion = phylumVersion
	})
}
