package shiroclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/sirupsen/logrus"
)

// WithHTTPClient allows specifying an overriding client for HTTP requests.
// This is helpful for testing.
func WithHTTPClient(client *http.Client) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.HTTPClient = client
	})
}

// WithLog allows specifying the logger to use.
func WithLog(log *logrus.Logger) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Log = log
	})
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.LogFields[key] = value
	})
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return types.Opt(func(r *types.RequestOptions) {
		for k, v := range fields {
			r.LogFields[k] = v
		}
	})
}

// WithHeader allows specifying an additional HTTP header.
func WithHeader(key string, value string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Headers[key] = value
	})
}

// WithEndpoint allows specifying the endpoint to target. The RPC
// implementation will not work if an endpoint is not specified.
func WithEndpoint(endpoint string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Endpoint = endpoint
	})
}

// WithID allows specifying the request ID. If the request ID is not
// specified, a randomly-generated UUID will be used.
func WithID(id string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.ID = id
	})
}

// WithParams allows specifying the phylum "parameters" argument. This
// must be set to something that json.Marshal accepts.
func WithParams(params interface{}) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Params = params
	})
}

// WithTransientData allows specifying a single "transient data"
// key-value pair.
func WithTransientData(key string, val []byte) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Transient[key] = val
	})
}

// WithTransientDataMap allows specifying multiple "transient data"
// key-value pairs.
func WithTransientDataMap(data map[string][]byte) Config {
	return types.Opt(func(r *types.RequestOptions) {
		for key, val := range data {
			r.Transient[key] = val
		}
	})
}

// WithResponse allows capturing the RPC response for futher analysis.
func WithResponse(target *interface{}) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Target = target
	})
}

// WithAuthToken passes authorization for the transaction issuer with a
// request.
func WithAuthToken(token string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.AuthToken = token
	})
}

// WithTimestampGenerator allows specifying a function that will be
// invoked at every Init or Call whose output is used to set the
// substrate "now" timestamp in mock mode. Has no effect outside of
// mock mode.
func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.TimestampGenerator = timestampGenerator
	})
}

// WithMSPFilter allows specifying the MSP filter. Has no effect in
// mock mode.
func WithMSPFilter(mspFilter []string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.MspFilter = append([]string(nil), mspFilter...)
	})
}

// WithTargetEndpoints allows specifying which exact peers will be used
// to process the transaction. Specifcy a name or URL of the peer.
func WithTargetEndpoints(nameOrURL []string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.TargetEndpoints = append([]string(nil), nameOrURL...)
	})
}

// WithoutTargetEndpoints allows specifying which exact peers will not
// be used to process the transaction. Specifcy a name or URL of the peer.
func WithoutTargetEndpoints(nameOrURL []string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.NotTargetEndpoints = append([]string(nil), nameOrURL...)
	})
}

// WithMinEndorsers allows specifying the minimum number of endorsing
// peers. Has no effect in mock mode.
func WithMinEndorsers(minEndorsers int) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.MinEndorsers = minEndorsers
	})
}

// WithCreator allows specifying the creator. Only has effect in mock
// mode. Also works in gateway mock mode.
func WithCreator(creator string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.Creator = creator
	})
}

// WithDependentTxID allows specifying a dependency on a transaction ID. If
// set, the client will poll for the presence of that transaction before
// simulating the request on the peer with the transaction.
func WithDependentTxID(txID string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.DependentTxID = txID
	})
}

// WithDisableWritePolling allows disabling polling for full consensus after a
// write is committed.
func WithDisableWritePolling(disable bool) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.DisableWritePolling = disable
	})
}

// WithCCFetchURLDowngrade allows controlling https -> http downgrade,
// typically useful before proxying for ccfetchurl library.
func WithCCFetchURLDowngrade(downgrade bool) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.CcFetchURLDowngrade = downgrade
	})
}

// WithCCFetchURLProxy sets the proxy for ccfetchurl library.
func WithCCFetchURLProxy(proxy *url.URL) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.CcFetchURLProxy = proxy
	})
}

// WithSingleton is useful for creating new config types.Options that do not take
// arguments.
func WithSingleton() Config {
	return types.Opt(func(r *types.RequestOptions) {})
}

// WithDependentBlock allows specifying a dependency on a block.  If
// set, the client will poll for the presence of that block before
// simulating the request on the peer with the block.
func WithDependentBlock(block string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.DependentBlock = block
	})
}

// WithPhylumVersion allows set a specific version of the phylum to simulate
// the transaction on. This overrides the default version set in the gateway.
func WithPhylumVersion(phylumVersion string) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.PhylumVersion = phylumVersion
	})
}

// WithResponseReceiver allows retrieving the shiro response directly.
func WithResponseReceiver(get func(resp ShiroResponse)) Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.ResponseReceiver = get
	})
}

// WithUnsafeDebug prints raw shiro-rpc responses to the logs.
func WithUnsafeDebug() Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.DebugPrint = true
	})
}
