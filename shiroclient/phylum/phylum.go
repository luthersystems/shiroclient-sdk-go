// Copyright © 2024 Luther Systems, Ltd. All right reserved.

// Package phylum provides a simple way to interact with shiroclient.
package phylum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	healthcheck "buf.build/gen/go/luthersystems/protos/protocolbuffers/go/healthcheck/v1"
	"github.com/luthersystems/shiroclient-sdk-go/internal/yaml2json"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/private"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// BootstrapProperty is the property name used to bootstrap the phylum.
const BootstrapProperty = "bootstrap-cfg"

// Config is an alias (not a distinct type)
type Config = shiroclient.Config

// defaultConfigs is used by the client as the starting config for most phylum
// calls.
var defaultConfigs = []func() (Config, error){
	private.WithSeed,
}

func joinConfig(base []func() (Config, error), add []Config) (conf []Config, err error) {
	nbase := len(base)
	conf = make([]Config, nbase+len(add))
	for i := range defaultConfigs {
		conf[i], err = defaultConfigs[i]()
		if err != nil {
			return nil, fmt.Errorf("default shiroclient config %d: %w", i, err)
		}
	}
	copy(conf[nbase:], add)
	return conf, nil
}

// cmdParams is a helper to construct positional arguments to pass to a shiro cmd.
func cmdParams(params ...proto.Message) []interface{} {
	if len(params) == 0 {
		return []interface{}{}
	}
	m := &protojson.MarshalOptions{UseProtoNames: true}
	jsparams := make([]interface{}, len(params))
	for i, p := range params {
		jsparams[i] = &jsProtoMessage{
			Message: p,
			m:       m,
		}
	}
	return jsparams
}

type jsProtoMessage struct {
	proto.Message
	m *protojson.MarshalOptions
}

func (msg *jsProtoMessage) MarshalJSON() ([]byte, error) {
	b, err := msg.m.Marshal(msg.Message)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Client is a phylum client.
type Client struct {
	log            *logrus.Entry
	rpc            shiroclient.ShiroClient
	GetLogMetadata func(context.Context) logrus.Fields
	closeFunc      func() error
}

// New returns a new phylum client.
func New(endpoint string, log *logrus.Entry) (*Client, error) {
	opts := []Config{
		shiroclient.WithEndpoint(endpoint),
		shiroclient.WithLogrusFields(log.Data),
	}
	client := &Client{
		log: log,
		rpc: shiroclient.NewRPC(opts),
	}
	return client, nil
}

// NewMock returns a mock phylum client.
func NewMock(phylumPath string, log *logrus.Entry) (*Client, error) {
	return NewMockFrom(phylumPath, log, nil)
}

// NewMockWithConfig returns a mock phylum client initialized with a optional bootstrap yaml.
func NewMockWithConfig(phylumPath string, log *logrus.Entry, bootstrapYAMLPath string) (*Client, error) {
	return newMockFrom(phylumPath, log, nil, bootstrapYAMLPath)
}

// NewMockFrom returns a mock phylum client restored from a DB snapshot.
func NewMockFrom(phylumPath string, log *logrus.Entry, r io.Reader) (*Client, error) {
	return newMockFrom(phylumPath, log, r, "")
}

// newMockFrom returns a mock phylum client restored from a DB snapshot.
func newMockFrom(phylumPath string, log *logrus.Entry, r io.Reader, cfgPath string) (*Client, error) {
	clientOpts := []Config{
		shiroclient.WithLogrusFields(log.Data),
	}
	mockOpts := []mock.Option{
		mock.WithSnapshotReader(r),
	}
	mock, err := shiroclient.NewMock(clientOpts, mockOpts...)
	if err != nil {
		return nil, err
	}
	client := &Client{
		log:       log,
		rpc:       mock,
		closeFunc: mock.Close,
	}

	if r != nil {
		// nothing more to do for snapshot flow, already boostrapped and initialized
		return client, nil
	}

	ctx := context.Background()

	if cfgPath != "" {
		// IMPORTANT: set the bootstrap *before* calling init, since the
		// user init function likely will use bootstrap data.
		jsonCfgBytes, err := yaml2json.JSONFromYAMLFile(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
		}

		// Call the phylum method to apply the bootstrap configuration.
		if err := client.SetAppControlProperty(ctx, BootstrapProperty, string(jsonCfgBytes)); err != nil {
			return nil, fmt.Errorf("failed to apply bootstrap config: %w", err)
		}
	}

	err = mock.Init(ctx, shiroclient.EncodePhylumBytes([]byte(phylumPath)))
	if err != nil {
		return nil, err
	}

	return client, nil
}

// shiroCall is a helper to make RPC calls.
func (s *Client) sdkCall(ctx context.Context, cmd string, params interface{}, rep proto.Message, clientConfigs []Config) error {
	clientConfigs, err := joinConfig(defaultConfigs, clientConfigs)
	if err != nil {
		return err
	}
	configs := make([]Config, 0, len(clientConfigs)+2)
	configs = append(configs, shiroclient.WithParams(params))
	configs = append(configs, clientConfigs...)
	resp, err := s.rpc.Call(ctx, cmd, configs...)
	if err != nil {
		if shiroclient.IsTimeoutError(err) {
			s.logEntry(ctx).WithError(err).Errorf("shiroclient timeout")
			return status.Error(codes.Unavailable, "timeout in blockchain network")
		}
		return err
	}
	if e := resp.Error(); e != nil {
		// json-rpc protocol error
		s.logEntry(ctx).WithFields(logrus.Fields{
			"cmd":          cmd,
			"jsonrpc_code": e.Code(),
			// IMPORTANT: we cannot log this since it may contain PII.
			//"jsonrpc_data":    string(jsonResp),
			"jsonrpc_message": e.Message(),
		}).Errorf("json-rpc error received from phylum")
		// Attempt to extract an error message string in the JSON
		// response, and bubble up an error that can be displayed on the
		// frontend. This allows `route-failure` string responses to be
		// displayed on the frontend.
		if ejs := e.DataJSON(); ejs != nil {
			var errMsg string
			err := json.Unmarshal(ejs, &errMsg)
			if err == nil {
				return errors.New(errMsg)
			}
		}
		// The error data wasn't a JSON string message, revert to a masked
		// error to avoid potentially leaking senstive/confusing objects to the
		// frontend.
		return fmt.Errorf("unknown phylum error")
	}
	if rep == nil || len(resp.ResultJSON()) == 0 || string(resp.ResultJSON()) == "null" {
		// nothing to unmarshal
		return nil
	}

	err = resp.UnmarshalTo(rep)
	if err != nil {
		s.logEntry(ctx).
			// IMPORTANT: we cannot log this since it may contain PII.
			// WithField("debug_json", string(resp.ResultJSON())).
			WithError(err).Errorf("Shiro RPC result could not be decoded")
		return err
	}

	return nil
}

// MockSnapshot copies the current state of the mock backend out to the supplied
// io.Writer.
func (s *Client) MockSnapshot(w io.Writer) error {
	mock, ok := s.rpc.(shiroclient.MockShiroClient)
	if !ok {
		return fmt.Errorf("client rpc does not not support snapshots")
	}
	return mock.Snapshot(w)
}

// Close closes the client if necessary.
func (s *Client) Close() error {
	if s.closeFunc == nil {
		return nil
	}
	return s.closeFunc()
}

func (s *Client) logFields(ctx context.Context) logrus.Fields {
	if s.GetLogMetadata == nil {
		return nil
	}
	return s.GetLogMetadata(ctx)
}

func (s *Client) logEntry(ctx context.Context) *logrus.Entry {
	return s.log.WithFields(s.logFields(ctx))
}

// HealthCheck performs health check on phylum.
func (s *Client) GetHealthCheck(ctx context.Context, services []string, config ...Config) (*healthcheck.GetHealthCheckResponse, error) {
	resp, err := shiroclient.RemoteHealthCheck(ctx, s.rpc, services, config...)
	if err != nil {
		return nil, err
	}
	return convertHealthResponse(resp), nil
}

func convertHealthResponse(health shiroclient.HealthCheck) *healthcheck.GetHealthCheckResponse {
	reports := health.Reports()
	healthpb := &healthcheck.GetHealthCheckResponse{
		Reports: make([]*healthcheck.HealthCheckReport, len(reports)),
	}
	for i, report := range reports {
		healthpb.Reports[i] = convertHealthReport(report)
	}
	return healthpb
}

func convertHealthReport(report shiroclient.HealthCheckReport) *healthcheck.HealthCheckReport {
	return &healthcheck.HealthCheckReport{
		Timestamp:      report.Timestamp(),
		Status:         report.Status(),
		ServiceName:    report.ServiceName(),
		ServiceVersion: report.ServiceVersion(),
	}
}

// Call sends requests to the phlyum, and returns a response.
func Call[K proto.Message, R proto.Message](s *Client, ctx context.Context, methodName string, req K, resp R, config ...Config) (R, error) {
	err := s.sdkCall(ctx, methodName, cmdParams(req), resp, config)
	if err != nil {
		var empty R
		return empty, err
	}
	return resp, nil
}

// SetAppControlProperty sets an application control property on the phylum.
// It encodes the provided value and calls the underlying "set_app_control_property" RPC.
// It returns an error if the call fails.
func (c *Client) SetAppControlProperty(ctx context.Context, name string, value string, configs ...shiroclient.Config) error {
	encodedValue := shiroclient.EncodePhylumBytes([]byte(value))
	params := []interface{}{name, encodedValue}
	// Call the underlying method using sdkCall. We don't expect any response, so rep is nil.
	if err := c.sdkCall(ctx, "set_app_control_property", params, nil, configs); err != nil {
		return fmt.Errorf("failed to set app control property %q: %w", name, err)
	}
	return nil
}

// GetAppControlProperty retrieves an application control property from the phylum.
// It sends the property name and expects a response containing a string value wrapped
// in a wrapperspb.StringValue. Returns the property value or an error.
func (c *Client) GetAppControlProperty(ctx context.Context, name string, configs ...shiroclient.Config) (string, error) {
	params := []interface{}{name}
	// Use wrapperspb.StringValue to hold the response value.
	response := &wrapperspb.StringValue{}
	if err := c.sdkCall(ctx, "get_app_control_property", params, response, configs); err != nil {
		return "", fmt.Errorf("failed to get app control property %q: %w", name, err)
	}
	return response.GetValue(), nil
}
