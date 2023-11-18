package shiroclient

import (
	"context"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/rpc"
	"github.com/sirupsen/logrus"
)

// WithContext allows specifying the context to use.
func WithContext(ctx context.Context) Config {
	return rpc.WithContext(ctx)
}

// WithLog allows specifying the logger to use.
func WithLog(log *logrus.Logger) Config {
	return rpc.WithLog(log)
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return rpc.WithLogField(key, value)
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return rpc.WithLogrusFields(fields)
}

// WithHeader allows specifying an additional HTTP header.
func WithHeader(key string, value string) Config {
	return rpc.WithHeader(key, value)
}

// WithEndpoint allows specifying the endpoint to target. The RPC
// implementation will not work if an endpoint is not specified.
func WithEndpoint(endpoint string) Config {
	return rpc.WithEndpoint(endpoint)
}

// WithID allows specifying the request ID. If the request ID is not
// specified, a randomly-generated UUID will be used.
func WithID(id string) Config {
	return rpc.WithID(id)
}

// WithParams allows specifying the phylum "parameters" argument. This
// must be set to something that json.Marshal accepts.
func WithParams(params interface{}) Config {
	return rpc.WithParams(params)
}

// WithTransientData allows specifying a single "transient data"
// key-value pair.
func WithTransientData(key string, val []byte) Config {
	return rpc.WithTransientData(key, val)
}

// WithTransientDataMap allows specifying multiple "transient data"
// key-value pairs.
func WithTransientDataMap(data map[string][]byte) Config {
	return rpc.WithTransientDataMap(data)
}

// WithResponse allows capturing the RPC response for futher analysis.
func WithResponse(target *interface{}) Config {
	return rpc.WithResponse(target)
}

// WithAuthToken passes authorization for the transaction issuer with a request
func WithAuthToken(token string) Config {
	return rpc.WithAuthToken(token)
}

// WithTimestampGenerator allows specifying a function that will be
// invoked at every Init or Call whose output is used to set the
// substrate "now" timestamp in mock mode. Has no effect outside of
// mock mode.
func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return rpc.WithTimestampGenerator(timestampGenerator)
}

// WithMSPFilter allows specifying the MSP filter. Has no effect in
// mock mode.
func WithMSPFilter(mspFilter []string) Config {
	return rpc.WithMSPFilter(mspFilter)
}

// WithMinEndorsers allows specifying the minimum number of endorsing
// peers. Has no effect in mock mode.
func WithMinEndorsers(minEndorsers int) Config {
	return rpc.WithMinEndorsers(minEndorsers)
}

// WithCreator allows specifying the creator. Only has effect in mock
// mode. Also works in gateway mock mode.
func WithCreator(creator string) Config {
	return rpc.WithCreator(creator)
}

// WithDependentTxID allows specifying a dependency on a transaction ID.  If
// set, the client will poll for the presence of that transaction before
// simulating the request on the peer with the transaction.
func WithDependentTxID(txID string) Config {
	return rpc.WithDependentTxID(txID)
}

// WithDisableWritePolling allows disabling polling for full consensus after a
// write is committed.
func WithDisableWritePolling(disable bool) Config {
	return rpc.WithDisableWritePolling(disable)
}

// WithCCFetchURLDowngrade allows controlling https -> http downgrade,
// typically useful before proxying for ccfetchurl library.
func WithCCFetchURLDowngrade(downgrade bool) Config {
	return rpc.WithCCFetchURLDowngrade(downgrade)
}

// WithCCFetchURLProxy sets the proxy for ccfetchurl library.
func WithCCFetchURLProxy(proxy *url.URL) Config {
	return rpc.WithCCFetchURLProxy(proxy)
}

// WithSingleton creates a config that does not do anything. This is useful
// for creating singleton configs in other packages (such as private).
func WithSingleton() Config {
	return rpc.WithSingleton()
}
