// Package shiroclient provides the ShiroClient interface and one
// implementations - a mode that connects to a JSON-RPC/HTTP gateway.
package shiroclient

import (
	"context"
	"encoding/base64"
	"fmt"

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
//	for _, report := range healthcheck {
//		if report.Status != "UP" {
//			ringAlarm(report)
//		}
//	}
type HealthCheckReport = rpc.HealthCheckReport

// IsTimeoutError inspects an error returned from shiroclient and returns true
// if it's a timeout.
func IsTimeoutError(err error) bool {
	return rpc.IsTimeoutError(err)
}

// OutcomeUnknownError indicates that a submitted transaction may still commit.
// Check the ledger for TxID before retrying. Old servers never produce this error.
type OutcomeUnknownError = rpc.OutcomeUnknownError

// ErrOutcomeUnknown matches ambiguous transaction outcomes via errors.Is.
// The transaction may still commit; check the ledger for its TxID before retrying.
// Old servers never report this state.
var ErrOutcomeUnknown = rpc.ErrOutcomeUnknown

// OutcomeUnknownTxID returns the transaction ID for an ambiguous outcome, including
// through wrapped errors. The transaction may still commit; check the ledger for
// TxID before retrying. The ID may be empty even when ok is true. Old servers never
// produce this state.
func OutcomeUnknownTxID(err error) (txID string, ok bool) {
	return rpc.OutcomeUnknownTxID(err)
}

// CallBatchRequest is one request of a CallBatch: a phylum method, its
// parameters and an optional JSON-RPC id.
type CallBatchRequest = types.CallBatchRequest

// CallBatchResponse is the result of a CallBatch: one response per request, in
// request order, and the batch's single transaction.
type CallBatchResponse = types.CallBatchResponse

// CallBatchError is the error CallBatch returns, together with the
// CallBatchResponse, when a request failed and so nothing was committed.  It
// names the failed request's index and id, and wraps that request's error.
type CallBatchError = types.CallBatchError

// CallBatcher is implemented by clients that support CallBatch; NewRPC and
// NewMock clients do.  It is not part of ShiroClient, so that adding it did
// not break other implementations of that interface.
type CallBatcher = types.CallBatcher

// ErrCallBatchNotSupported matches, via errors.Is, a CallBatch that could not run
// at all: the client does not implement CallBatcher, the gateway predates
// luthersystems/substrate#521 ("method not found"), or the client is a mock
// (the substrate plugin does not support batches yet).  Nothing was run.
var ErrCallBatchNotSupported = types.ErrCallBatchNotSupported

// CallBatch runs several phylum methods as ONE transaction, all or nothing.
//
// Requests run in order and each sees the writes of the ones before it.  If
// every request succeeds and the batch wrote state, it is committed once:
// CallBatchResponse.Committed is true and TxID is the transaction's ID (a batch
// that only reads is not committed and has no TxID, like a read-only Call).
//
// If any request fails, NOTHING is committed.  CallBatch then returns the
// CallBatchResponse together with a *CallBatchError naming the failed request.  The
// failed request's response carries its own error; every other request's
// response carries a "batch aborted" error (see CallBatchAborted).
//
// The configs are those of Call, applied once to the whole batch.  Transient
// data (WithTransientData) is shared by every request, and every request runs
// against the same phylum version (WithPhylumVersion).  WithParams is
// ignored: each request carries its own Params.
//
// A timeout (IsTimeoutError) means the batch MAY have committed: CallBatch
// returns no CallBatchResponse and no CallBatchError, and the caller must
// reconcile against the ledger before running the requests again.  With a
// gateway that also includes luthersystems/substrate#515, the error also
// matches ErrOutcomeUnknown and OutcomeUnknownTxID returns the transaction
// ID to look for; an older gateway sends no transaction ID.  Never retry a
// timed-out batch as a new batch, or as separate Calls, without reconciling.
//
// CallBatch needs a shiroclient gateway that includes
// luthersystems/substrate#521.  With an older gateway, a mock client, or a
// client that does not implement CallBatcher, it returns an error matching
// ErrCallBatchNotSupported and runs nothing; it never falls back to separate
// Calls, which would not be atomic.
func CallBatch(ctx context.Context, client ShiroClient, requests []CallBatchRequest, configs ...Config) (*CallBatchResponse, error) {
	bc, ok := client.(CallBatcher)
	if !ok {
		return nil, fmt.Errorf("%w: %T does not implement CallBatcher", ErrCallBatchNotSupported, client)
	}
	return bc.CallBatch(ctx, requests, configs...)
}

// CodeBatchAborted is the JSON-RPC error code substrate reserves for a request
// of a batch that was not committed because another request failed. A phylum
// error never carries it.
const CodeBatchAborted = types.CodeBatchAborted

// CallBatchAborted reports whether err is the error given to a request of a
// batch that did not fail itself but was not committed because another
// request failed.  failedID is the id of the request that failed.
func CallBatchAborted(err Error) (failedID interface{}, ok bool) {
	return types.CallBatchAborted(err)
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
//
//	phylum
//	shiroclient_gateway
//	fabric_peer
//	...
//
// Unrecognized service names are ignored, though may still be sent to upstream
// gateways.
//
// NOTE:  An RPC gateway must be a recent enough version to support
// specification of upstream services or it will otherwise fallback to invoking
// the phylum healthcheck endpoint.
func RemoteHealthCheck(ctx context.Context, client ShiroClient, services []string, configs ...Config) (HealthCheck, error) {
	return rpc.RemoteHealthCheck(ctx, client, services, configs...)
}
