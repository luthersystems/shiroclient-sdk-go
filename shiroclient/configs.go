package shiroclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/trace"
	"github.com/sirupsen/logrus"
)

type standardConfig struct {
	fn func(*types.RequestOptions)
}

func (s *standardConfig) Fn(r *types.RequestOptions) {
	s.fn(r)
}

func opt(fn func(r *types.RequestOptions)) Config {
	return &standardConfig{fn}
}

// WithHTTPClient allows specifying an overriding client for HTTP requests.
// This is helpful for testing.
func WithHTTPClient(client *http.Client) Config {
	return opt(func(r *types.RequestOptions) {
		r.HTTPClient = client
	})
}

// WithContext allows specifying the context to use.
func WithContext(ctx context.Context) Config {
	return opt(func(r *types.RequestOptions) {
		r.Ctx = ctx
	})
}

// WithLog allows specifying the logger to use.
func WithLog(log *logrus.Logger) Config {
	return opt(func(r *types.RequestOptions) {
		r.Log = log
	})
}

// WithLogField allows specifying a log field to be included.
func WithLogField(key string, value interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.LogFields[key] = value
	})
}

// WithLogrusFields allows specifying multiple log fields to be
// included.
func WithLogrusFields(fields logrus.Fields) Config {
	return opt(func(r *types.RequestOptions) {
		for k, v := range fields {
			r.LogFields[k] = v
		}
	})
}

// WithHeader allows specifying an additional HTTP header.
func WithHeader(key string, value string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Headers[key] = value
	})
}

// WithEndpoint allows specifying the endpoint to target. The RPC
// implementation will not work if an endpoint is not specified.
func WithEndpoint(endpoint string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Endpoint = endpoint
	})
}

// WithID allows specifying the request ID. If the request ID is not
// specified, a randomly-generated UUID will be used.
func WithID(id string) Config {
	return opt(func(r *types.RequestOptions) {
		r.ID = id
	})
}

// WithParams allows specifying the phylum "parameters" argument. This
// must be set to something that json.Marshal accepts.
func WithParams(params interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.Params = params
	})
}

// WithTransientData allows specifying a single "transient data"
// key-value pair.
func WithTransientData(key string, val []byte) Config {
	return opt(func(r *types.RequestOptions) {
		r.Transient[key] = val
	})
}

// WithTransientDataMap allows specifying multiple "transient data"
// key-value pairs.
func WithTransientDataMap(data map[string][]byte) Config {
	return opt(func(r *types.RequestOptions) {
		for key, val := range data {
			r.Transient[key] = val
		}
	})
}

// WithResponse allows capturing the RPC response for futher analysis.
func WithResponse(target *interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.Target = target
	})
}

// WithAuthToken passes authorization for the transaction issuer with a
// request.
func WithAuthToken(token string) Config {
	return opt(func(r *types.RequestOptions) {
		r.AuthToken = token
	})
}

// WithTimestampGenerator allows specifying a function that will be
// invoked at every Init or Call whose output is used to set the
// substrate "now" timestamp in mock mode. Has no effect outside of
// mock mode.
func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return opt(func(r *types.RequestOptions) {
		r.TimestampGenerator = timestampGenerator
	})
}

// WithMSPFilter allows specifying the MSP filter. Has no effect in
// mock mode.
func WithMSPFilter(mspFilter []string) Config {
	return opt(func(r *types.RequestOptions) {
		r.MspFilter = append([]string(nil), mspFilter...)
	})
}

// WithMinEndorsers allows specifying the minimum number of endorsing
// peers. Has no effect in mock mode.
func WithMinEndorsers(minEndorsers int) Config {
	return opt(func(r *types.RequestOptions) {
		r.MinEndorsers = minEndorsers
	})
}

// WithCreator allows specifying the creator. Only has effect in mock
// mode. Also works in gateway mock mode.
func WithCreator(creator string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Creator = creator
	})
}

// WithDependentTxID allows specifying a dependency on a transaction ID. If
// set, the client will poll for the presence of that transaction before
// simulating the request on the peer with the transaction.
func WithDependentTxID(txID string) Config {
	return opt(func(r *types.RequestOptions) {
		r.DependentTxID = txID
	})
}

// WithDisableWritePolling allows disabling polling for full consensus after a
// write is committed.
func WithDisableWritePolling(disable bool) Config {
	return opt(func(r *types.RequestOptions) {
		r.DisableWritePolling = disable
	})
}

// WithCCFetchURLDowngrade allows controlling https -> http downgrade,
// typically useful before proxying for ccfetchurl library.
func WithCCFetchURLDowngrade(downgrade bool) Config {
	return opt(func(r *types.RequestOptions) {
		r.CcFetchURLDowngrade = downgrade
	})
}

// WithCCFetchURLProxy sets the proxy for ccfetchurl library.
func WithCCFetchURLProxy(proxy *url.URL) Config {
	return opt(func(r *types.RequestOptions) {
		r.CcFetchURLProxy = proxy
	})
}

// WithSingleton is useful for creating new config options that do not take
// arguments.
func WithSingleton() Config {
	return opt(func(r *types.RequestOptions) {})
}

// WithDependentBlock allows specifying a dependency on a block.  If
// set, the client will poll for the presence of that block before
// simulating the request on the peer with the block.
func WithDependentBlock(block string) Config {
	return opt(func(r *types.RequestOptions) {
		r.DependentBlock = block
	})
}

// WithPhylumVersion allows set a specific version of the phylum to simulate
// the transaction on. This overrides the default version set in the gateway.
func WithPhylumVersion(phylumVersion string) Config {
	return opt(func(r *types.RequestOptions) {
		r.PhylumVersion = phylumVersion
	})
}

// WithJaegerTracing sets options to enable tracing with Jaeger.  The
// collectorURI param is the full URI to the Jaeger HTTP Thrift collector.  For
// example, http://localhost:14268/api/traces.
func WithJaegerTracing(collectorURI, datasetID string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Transient[trace.TransientKeyJaegerCollectorURI] = []byte(collectorURI)
		r.Transient[trace.TransientKeyDatasetID] = []byte(datasetID)
	})
}
