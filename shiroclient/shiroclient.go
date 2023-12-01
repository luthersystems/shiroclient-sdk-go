// Package shiroclient provides the ShiroClient interface and one
// implementations - a mode that connects to a JSON-RPC/HTTP gateway.
package shiroclient

import (
	"context"
	"encoding/base64"

	imock "github.com/luthersystems/shiroclient-sdk-go/internal/mock"
	"github.com/luthersystems/shiroclient-sdk-go/internal/rpc"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/mock"
)

// ShiroClient interfaces with blockchain-based smart contract execution engine.
// Currently, the "phylum" code must be written in a LISP dialect known as ELPS.
type ShiroClient = types.ShiroClient

// MockShiroClient is an abstraction for a ShiroClient that is backed
// by an in-process lightweight ledger. This uses the hashicorp plugin.
type MockShiroClient = imock.MockShiroClient

// Config is a type for a function that can mutate a types.RequestOptions
// object.
type Config = types.Config

// ShiroResponse is a wrapper for a response from a shiro
// chaincode. Even if the chaincode was invoked successfully, it may
// have signaled an error.
type ShiroResponse = types.ShiroResponse

// Error is a generic application error.
type Error types.Error

// Transaction has summary information about a transaction.
type Transaction types.Transaction

// Block has summary information about a block.
type Block = types.Block

// HealthCheck is a collection of reports detailing connectivity and health of
// system components (e.g. phylum, RPC gateway, etc).  See RemoteHealthCheck.
type HealthCheck = rpc.HealthCheck

// HealthCheckReport details the connectivity/health of an individual system
// component as part of a HealthCheck.  When inspecting HealthCheckReports a
// service should only be considered operational if its reported status is
// "UP".  Any other status indicates a potential service interruption.
//
// 		for _, report := range healthcheck {
//			if report.Status != "UP" {
//				ringAlarm(report)
//			}
//		}
//
type HealthCheckReport = rpc.HealthCheckReport

// IsTimeoutError inspects an error returned from shiroclient and returns true
// if it's a timeout.
func IsTimeoutError(err error) bool {
	return rpc.IsTimeoutError(err)
}

// NewRPC creates a new RPC ShiroClient with the given set of base
// configs that will be applied to all commands.
func NewRPC(clientConfigs []Config) ShiroClient {
	return rpc.NewRPC(clientConfigs)
}

// NewMock creates a new mock ShiroClient with the given set of base
// configs that will be applied to all commands.
func NewMock(clientConfigs []Config, opts ...mock.Option) (MockShiroClient, error) {
	return imock.NewMock(clientConfigs, opts...)
}

// EncodePhylumBytes takes decoded phylum (lisp code) and encodes it
// for use with the Init() method.
func EncodePhylumBytes(decoded []byte) string {
	return base64.StdEncoding.EncodeToString(decoded)
}

// UnmarshalProto attempts to unmarshal protobuf bytes with backwards compatability.
func UnmarshalProto(src []byte, dst interface{}) error {
	return types.UnmarshalProto(src, dst)
}

// RemoteHealthCheck checks connectivity between the SDK client (e.g. oracle
// service) and upstream services including the phylum itself.  If the list of
// upstream services is empty the behavior of RemoteHealthCheck depends on
// client's implementation.  Clients created with NewMock do not support
// upstream service enumeration and will always invoke the mock phylum
// "healthcheck" endpoint.
//
// For clients that support RemoteHealthCheck service enumeration, like those
// created with NewRPC, services should be specified using canonical names
//		phylum
//		shiroclient_gateway
//		fabric_peer
//		...
// Unrecognized service names are ignored, though may still be sent to upstream
// gateways.
//
// NOTE:  An RPC gateway must be a recent enough version to support
// specification of upstream services or it will otherwise fallback to invoking
// the phylum healthcheck endpoint.
func RemoteHealthCheck(ctx context.Context, client ShiroClient, services []string, configs ...Config) (HealthCheck, error) {
	return rpc.RemoteHealthCheck(ctx, client, services, configs...)
}
