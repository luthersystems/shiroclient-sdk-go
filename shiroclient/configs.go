package shiroclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/types"
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

func WithHTTPClient(client *http.Client) Config {
	return opt(func(r *types.RequestOptions) {
		r.HTTPClient = client
	})
}

func WithContext(ctx context.Context) Config {
	return opt(func(r *types.RequestOptions) {
		r.Ctx = ctx
	})
}

func WithLog(log *logrus.Logger) Config {
	return opt(func(r *types.RequestOptions) {
		r.Log = log
	})
}

func WithLogField(key string, value interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.LogFields[key] = value
	})
}

func WithLogrusFields(fields logrus.Fields) Config {
	return opt(func(r *types.RequestOptions) {
		for k, v := range fields {
			r.LogFields[k] = v
		}
	})
}

func WithHeader(key string, value string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Headers[key] = value
	})
}

func WithEndpoint(endpoint string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Endpoint = endpoint
	})
}

func WithID(id string) Config {
	return opt(func(r *types.RequestOptions) {
		r.ID = id
	})
}

func WithParams(params interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.Params = params
	})
}

func WithTransientData(key string, val []byte) Config {
	return opt(func(r *types.RequestOptions) {
		r.Transient[key] = val
	})
}

func WithTransientDataMap(data map[string][]byte) Config {
	return opt(func(r *types.RequestOptions) {
		for key, val := range data {
			r.Transient[key] = val
		}
	})
}

func WithResponse(target *interface{}) Config {
	return opt(func(r *types.RequestOptions) {
		r.Target = target
	})
}

func WithAuthToken(token string) Config {
	return opt(func(r *types.RequestOptions) {
		r.AuthToken = token
	})
}

func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return opt(func(r *types.RequestOptions) {
		r.TimestampGenerator = timestampGenerator
	})
}

func WithMSPFilter(mspFilter []string) Config {
	return opt(func(r *types.RequestOptions) {
		r.MspFilter = append([]string(nil), mspFilter...)
	})
}

func WithMinEndorsers(minEndorsers int) Config {
	return opt(func(r *types.RequestOptions) {
		r.MinEndorsers = minEndorsers
	})
}

func WithCreator(creator string) Config {
	return opt(func(r *types.RequestOptions) {
		r.Creator = creator
	})
}

func WithDependentTxID(txID string) Config {
	return opt(func(r *types.RequestOptions) {
		r.DependentTxID = txID
	})
}

func WithDisableWritePolling(disable bool) Config {
	return opt(func(r *types.RequestOptions) {
		r.DisableWritePolling = disable
	})
}

func WithCCFetchURLDowngrade(downgrade bool) Config {
	return opt(func(r *types.RequestOptions) {
		r.CcFetchURLDowngrade = downgrade
	})
}

func WithCCFetchURLProxy(proxy *url.URL) Config {
	return opt(func(r *types.RequestOptions) {
		r.CcFetchURLProxy = proxy
	})
}

func WithSingleton() Config {
	return opt(func(r *types.RequestOptions) {})
}

func WithDependentBlock(block string) Config {
	return opt(func(r *types.RequestOptions) {
		r.DependentBlock = block
	})
}

func WithPhylumVersion(phylumVersion string) Config {
	return opt(func(r *types.RequestOptions) {
		r.PhylumVersion = phylumVersion
	})
}
