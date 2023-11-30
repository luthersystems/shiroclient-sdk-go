package shiroclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/types"
	"github.com/sirupsen/logrus"
)

// WithHTTPClient allows specifying an http client for RPC calls.
func WithHTTPClient(client *http.Client) Config {
	return types.WithHTTPClient(client)
}

// WithContext allows specifying the context to use.
func WithContext(ctx context.Context) Config {
	return types.WithContext(ctx)
}

// WithLog allows specifying the logger to use.
func WithLog(log *logrus.Logger) Config {
	return types.WithLog(log)
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return types.WithLogField(key, value)
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return types.WithLogrusFields(fields)
}

// WithHeader allows specifying an additional HTTP header.
func WithHeader(key string, value string) Config {
	return types.WithHeader(key, value)
}

// WithEndpoint allows specifying the endpoint to target. The RPC
// implementation will not work if an endpoint is not specified.
func WithEndpoint(endpoint string) Config {
	return types.WithEndpoint(endpoint)
}

// WithID allows specifying the request ID. If the request ID is not
// specified, a randomly-generated UUID will be used.
func WithID(id string) Config {
	return types.WithID(id)
}

// WithParams allows specifying the phylum "parameters" argument. This
// must be set to something that json.Marshal accepts.
func WithParams(params interface{}) Config {
	return types.WithParams(params)
}

// WithTransientData allows specifying a single "transient data"
// key-value pair.
func WithTransientData(key string, val []byte) Config {
	return types.WithTransientData(key, val)
}

// WithTransientDataMap allows specifying multiple "transient data"
// key-value pairs.
func WithTransientDataMap(data map[string][]byte) Config {
	return types.WithTransientDataMap(data)
}

// WithResponse allows capturing the RPC response for futher analysis.
func WithResponse(target *interface{}) Config {
	return types.WithResponse(target)
}

// WithAuthToken passes authorization for the transaction issuer with a request
func WithAuthToken(token string) Config {
	return types.WithAuthToken(token)
}

// WithTimestampGenerator allows specifying a function that will be
// invoked at every Init or Call whose output is used to set the
// substrate "now" timestamp in mock mode. Has no effect outside of
// mock mode.
func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return types.WithTimestampGenerator(timestampGenerator)
}

// WithMSPFilter allows specifying the MSP filter. Has no effect in
// mock mode.
func WithMSPFilter(mspFilter []string) Config {
	return types.WithMSPFilter(mspFilter)
}

// WithMinEndorsers allows specifying the minimum number of endorsing
// peers. Has no effect in mock mode.
func WithMinEndorsers(minEndorsers int) Config {
	return types.WithMinEndorsers(minEndorsers)
}

// WithCreator allows specifying the creator. Only has effect in mock
// mode. Also works in gateway mock mode.
func WithCreator(creator string) Config {
	return types.WithCreator(creator)
}

// WithDependentTxID allows specifying a dependency on a transaction ID.  If
// set, the client will poll for the presence of that transaction before
// simulating the request on the peer with the transaction.
func WithDependentTxID(txID string) Config {
	return types.WithDependentTxID(txID)
}

// WithDisableWritePolling allows disabling polling for full consensus after a
// write is committed.
func WithDisableWritePolling(disable bool) Config {
	return types.WithDisableWritePolling(disable)
}

// WithCCFetchURLDowngrade allows controlling https -> http downgrade,
// typically useful before proxying for ccfetchurl library.
func WithCCFetchURLDowngrade(downgrade bool) Config {
	return types.WithCCFetchURLDowngrade(downgrade)
}

// WithCCFetchURLProxy sets the proxy for ccfetchurl library.
func WithCCFetchURLProxy(proxy *url.URL) Config {
	return types.WithCCFetchURLProxy(proxy)
}

// WithSingleton creates a config that does not do anything. This is useful
// for creating singleton configs in other packages (such as private).
func WithSingleton() Config {
	return types.WithSingleton()
}

// WithDependentBlock allows specifying a dependency on a block.  If
// set, the client will poll for the presence of that block before
// simulating the request on the peer with the block.
func WithDependentBlock(blockNum string) Config {
	return types.WithDependentBlock(blockNum)
}

// WithPhylumVersion allows specifying a specific phylum version.  If
// set, the client will override the shiroclient gateway phylum version, to
// use the specified version. The version must be installed and in ACTIVE
// state within substrate.
func WithPhylumVersion(phylumVersion string) Config {
	return types.WithPhylumVersion(phylumVersion)
}
