package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// ErrCallBatchNotSupported is returned by CallBatch when the client or the
// gateway cannot run a batch: a gateway older than luthersystems/substrate#521
// answers "method not found", and the mock plugin does not support batches
// yet.  Nothing was run.
var ErrCallBatchNotSupported = errors.New("shiroclient: CallBatch not supported")

// ErrQueryBatchNotSupported is returned by QueryBatch when the client or the
// gateway cannot run one: a gateway without QueryBatch answers "method not
// found", and the mock plugin does not support batches yet.  Nothing was
// run.
var ErrQueryBatchNotSupported = errors.New("shiroclient: QueryBatch not supported")

// BatchTransientPrefix is reserved: the gateway packs a CallBatch request's
// own transient keys as "$batch/<index>/<key>", and rejects a caller's key
// with this prefix in Call and CallBatch alike.
const BatchTransientPrefix = "$batch/"

// CheckTransientKeys rejects transient keys with the reserved
// BatchTransientPrefix.
func CheckTransientKeys(transient map[string][]byte) error {
	for k := range transient {
		if strings.HasPrefix(k, BatchTransientPrefix) {
			return fmt.Errorf("transient key %q: the %q prefix is reserved", k, BatchTransientPrefix)
		}
	}
	return nil
}

// TransactionTransientKeys are the transient keys that belong to the whole
// transaction rather than to one request: the CSPRNG seed, the timestamp
// override and the trace context.  They are the only transient keys a
// CallBatch shares between its requests, and a request cannot set them.
var TransactionTransientKeys = []string{"csprng_seed_private", "timestamp_override", "traceparent", "tracestate"}

// IsTransactionTransientKey reports whether key is one of
// TransactionTransientKeys.
func IsTransactionTransientKey(key string) bool {
	for _, k := range TransactionTransientKeys {
		if k == key {
			return true
		}
	}
	return false
}

// CheckBatchTransientKeys rejects shared CallBatch transient data other than
// the TransactionTransientKeys: a request's data goes in its own Configs.
func CheckBatchTransientKeys(transient map[string][]byte) error {
	for _, k := range sortedKeys(transient) {
		if !IsTransactionTransientKey(k) {
			return fmt.Errorf("transient key %q cannot be shared by a batch: only %s are; set request data in that CallBatchRequest's Configs",
				k, strings.Join(TransactionTransientKeys, ", "))
		}
	}
	return nil
}

// checkRequestTransientKeys rejects keys a request cannot set: empty keys,
// the reserved BatchTransientPrefix and the TransactionTransientKeys.
func checkRequestTransientKeys(transient map[string][]byte) error {
	for _, k := range sortedKeys(transient) {
		switch {
		case k == "":
			return errors.New("empty transient key")
		case IsTransactionTransientKey(k):
			return fmt.Errorf("transient key %q is transaction-wide: set it in CallBatch's configs, not a request's", k)
		}
	}
	return CheckTransientKeys(transient)
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// CSPRNGSeedKey is the transient key of a transaction's CSPRNG seed.
const CSPRNGSeedKey = "csprng_seed_private"

// csprngSeedConfig sets CSPRNGSeedKey.  It is a type of its own so that
// RequestTransient can recognise it in a request's Configs.
type csprngSeedConfig struct {
	seed []byte
}

// Fn implements Config.
func (c *csprngSeedConfig) Fn(r *RequestOptions) {
	r.Transient[CSPRNGSeedKey] = c.seed
}

// CSPRNGSeedConfig returns a Config that sets the CSPRNG seed transient
// key.  A transaction has one seed, so in a CallBatchRequest's Configs this
// config does not set the request's transient data: RequestTransient
// returns the seed instead, and the batch uses it as the transaction's seed
// when the batch's own configs set none and no earlier request carried one.
func CSPRNGSeedConfig(seed []byte) Config {
	return &csprngSeedConfig{seed: seed}
}

// CallBatcher is implemented by clients that can run several phylum methods
// as one all-or-nothing transaction.  It is separate from ShiroClient so that
// adding it did not break other implementations of that interface.
type CallBatcher interface {
	CallBatch(ctx context.Context, requests []CallBatchRequest, config ...Config) (*CallBatchResponse, error)
}

// QueryBatcher is implemented by clients that can simulate several phylum
// methods as one all-or-nothing transaction that is never committed.
type QueryBatcher interface {
	QueryBatch(ctx context.Context, requests []CallBatchRequest, config ...Config) (*CallBatchResponse, error)
}

// CallBatchRequest is one request of a CallBatch or a QueryBatch.
type CallBatchRequest struct {
	// Params are the method's parameters, which must encode to a JSON array
	// or object.  Anything that encodes to null (nil, or a nil slice, map or
	// pointer) is sent as an empty array; any other value is rejected before
	// the batch is sent.
	Params interface{}
	// ID is the request's JSON-RPC id: nil, a string or a number (a Go
	// integer or float type, or json.Number).  Other types are rejected
	// before the batch is sent.  When nil, the server uses the request's
	// index in the batch.
	ID interface{}
	// Configs are Call configs for this request only, applied in order as
	// for Call.  Only transient data may be set here: WithTransientData,
	// WithTransientDataMap and helpers built on them, such as
	// private.WithTransientMXF.  That data is sent with this request alone.
	// It routes the data to this request; it does not hide it from phylum
	// code or from endorsing peers, which receive every request's data.
	//
	// Keys must be non-empty, must not start with "$batch/", and must not
	// be a transaction-wide key (TransactionTransientKeys), which belongs in
	// the batch's own configs.  The CSPRNG seed that private.WithSeed and
	// private.WithTransientMXF carry is the exception: it is promoted to the
	// transaction's one seed when the batch's own configs set none and no
	// earlier request carried one, and is ignored otherwise.
	//
	// Every other option applies to the whole transaction or to the HTTP
	// call and is refused before the batch is sent, naming the option:
	// WithParams and WithID (use Params and ID), WithEndpoint, WithHeader,
	// WithAuthToken, WithHTTPClient, WithLog, WithLogField,
	// WithLogrusFields, WithMSPFilter, WithTargetEndpoints,
	// WithoutTargetEndpoints, WithMinEndorsers, WithCreator,
	// WithDependentTxID, WithDependentBlock, WithPhylumVersion,
	// WithDisableWritePolling, WithTimestampGenerator, WithCCFetchURLProxy,
	// WithCCFetchURLDowngrade, WithResponse, WithResponseReceiver and
	// WithUnsafeDebug.  Pass those in the batch's own configs.
	Configs []Config
	// Method is the phylum endpoint to call.
	Method string
}

// CallBatchResponse is the result of a CallBatch or a QueryBatch.  A
// QueryBatch is never committed: Committed is false and TxID is empty.
type CallBatchResponse struct {
	// Responses holds one response per request, in request order.  In a
	// batch that was not committed because a request failed, every response
	// is a failure.
	Responses []ShiroResponse
	// IDs holds each response's JSON-RPC id as the server returned it,
	// decoded from JSON (so a numeric id is a float64).
	IDs []interface{}
	// TxID is the committed transaction's ID; empty when not committed.
	TxID string
	// CommitBlockNum is the block that committed the transaction, or 0.
	CommitBlockNum uint64
	// MaxSimBlockNum is the max block number that simulated the batch.
	MaxSimBlockNum uint64
	// Committed reports whether the batch was committed: it wrote state and
	// no request failed.  A batch that only read is not committed.
	Committed bool
}

// FailedIndex returns the index of the request whose own failure spoiled the
// batch, or -1 when no request failed.  When no failure is attributable to a
// single request, the first failed request is returned.
func (r *CallBatchResponse) FailedIndex() int {
	if r == nil {
		return -1
	}
	first := -1
	for i, resp := range r.Responses {
		if resp == nil || resp.Error() == nil {
			continue
		}
		if first < 0 {
			first = i
		}
		if _, aborted := CallBatchAborted(resp.Error()); !aborted {
			return i
		}
	}
	return first
}

// CallBatchError is returned, together with the CallBatchResponse, when a
// request of a CallBatch or QueryBatch failed and so nothing was committed
// and the other requests' results are void.
type CallBatchError struct {
	// ID is the failed request's JSON-RPC id, as in CallBatchResponse.IDs.
	ID interface{}
	// Err is the failed request's own error.
	Err Error
	// Index is the failed request's index in the batch.
	Index int
}

// Error implements error.
func (e *CallBatchError) Error() string {
	msg := "unknown error"
	if e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("shiroclient: batch not committed: request %d (id %v) failed: %s", e.Index, e.ID, msg)
}

// Unwrap returns the failed request's error.
func (e *CallBatchError) Unwrap() error {
	if e.Err == nil {
		return nil
	}
	return e.Err
}

// CodeBatchAborted is the JSON-RPC error code substrate reserves for a request
// of a batch that was not run, or not committed, because another request
// failed. The router never lets a phylum error carry it.
const CodeBatchAborted = -32001

// CodeForcedNoCommit is the JSON-RPC error code substrate gives, in a
// CallBatch, to a request whose method forces its transaction not to commit
// (private_decode, for example: it writes only to decode, and those writes
// must be discarded).  Such a request cannot share a transaction that is
// meant to commit, so it fails the batch: it is the CallBatchError's
// request, and every other request gets CodeBatchAborted.  Run such
// requests with QueryBatch, which never commits.
const CodeForcedNoCommit = -32002

// CallBatchAborted reports whether err is the error given to a request of a batch
// that did not fail itself but was not committed because another request
// failed.  failedID is the id of the request that failed. Only the reserved
// code CodeBatchAborted counts: a phylum error whose data merely looks like
// the abort marker is that request's own failure.
func CallBatchAborted(err Error) (failedID interface{}, ok bool) {
	if err == nil || err.Code() != CodeBatchAborted {
		return nil, false
	}
	var data struct {
		FailedID         interface{} `json:"failed_id"`
		CallBatchAborted bool        `json:"batch_aborted"`
	}
	if json.Unmarshal(err.DataJSON(), &data) != nil || !data.CallBatchAborted {
		return nil, false
	}
	return data.FailedID, true
}

// requestOptionNames names the Call configs that set each RequestOptions
// field, for the error that refuses them in CallBatchRequest.Configs.  A
// field missing here is refused too, under its field name.
var requestOptionNames = map[string]string{
	"Params":              "WithParams (use CallBatchRequest.Params)",
	"ID":                  "WithID (use CallBatchRequest.ID)",
	"Target":              "WithResponse",
	"Log":                 "WithLog",
	"LogFields":           "WithLogField/WithLogrusFields",
	"Headers":             "WithHeader",
	"CcFetchURLProxy":     "WithCCFetchURLProxy",
	"HTTPClient":          "WithHTTPClient",
	"TimestampGenerator":  "WithTimestampGenerator",
	"Endpoint":            "WithEndpoint",
	"NewPhylumVersion":    "the new phylum version",
	"PhylumVersion":       "WithPhylumVersion",
	"DependentBlock":      "WithDependentBlock",
	"AuthToken":           "WithAuthToken",
	"Creator":             "WithCreator",
	"DependentTxID":       "WithDependentTxID",
	"NotTargetEndpoints":  "WithoutTargetEndpoints",
	"TargetEndpoints":     "WithTargetEndpoints",
	"MspFilter":           "WithMSPFilter",
	"MinEndorsers":        "WithMinEndorsers",
	"DisableWritePolling": "WithDisableWritePolling",
	"CcFetchURLDowngrade": "WithCCFetchURLDowngrade",
	"ResponseReceiver":    "WithResponseReceiver",
	"DebugPrint":          "WithUnsafeDebug",
}

// RequestTransient applies a CallBatchRequest's Configs and returns the
// transient data they set, or nil when they set none.  Every other option
// is refused: it applies to the whole transaction or the HTTP call, not to
// one request.  A new RequestOptions field is refused until it is known to
// be per-request.  A CSPRNGSeedConfig (private.WithSeed, and so
// private.WithTransientMXF) does not set the request's transient data: its
// seed is returned, the first one when there are several, for the batch to
// promote to the transaction's seed.
func RequestTransient(configs []Config) (transient map[string][]byte, seed []byte, err error) {
	if len(configs) == 0 {
		return nil, nil, nil
	}
	newOpts := func() *RequestOptions {
		return &RequestOptions{
			LogFields: map[string]interface{}{},
			Headers:   map[string]string{},
			Transient: map[string][]byte{},
		}
	}
	opt, blank := newOpts(), newOpts()
	for i, c := range configs {
		if c == nil {
			return nil, nil, fmt.Errorf("config %d is nil", i)
		}
		if sc, ok := c.(*csprngSeedConfig); ok {
			if seed == nil {
				seed = sc.seed
			}
			continue
		}
		c.Fn(opt)
	}
	got, want := reflect.ValueOf(opt).Elem(), reflect.ValueOf(blank).Elem()
	for i := 0; i < got.NumField(); i++ {
		name := got.Type().Field(i).Name
		if name == "Transient" {
			continue
		}
		// DeepEqual treats two nil funcs as equal and any non-nil func as
		// set, which is what refusing a func option needs.
		if !reflect.DeepEqual(got.Field(i).Interface(), want.Field(i).Interface()) {
			label, ok := requestOptionNames[name]
			if !ok {
				label = name
			}
			return nil, nil, fmt.Errorf("%s cannot be set per request: it applies to the whole transaction or HTTP call; pass it in CallBatch's configs", label)
		}
	}
	if len(opt.Transient) == 0 {
		return nil, seed, nil
	}
	if err := checkRequestTransientKeys(opt.Transient); err != nil {
		return nil, nil, err
	}
	return opt.Transient, seed, nil
}
