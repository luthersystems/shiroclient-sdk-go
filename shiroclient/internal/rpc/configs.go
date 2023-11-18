package rpc

import (
	"context"
	"net/url"

	"github.com/sirupsen/logrus"
)

type standardConfig struct {
	fn func(*RequestOptions)
}

func (s *standardConfig) Fn(r *RequestOptions) {
	s.fn(r)
}

func opt(fn func(r *RequestOptions)) Config {
	return &standardConfig{fn}
}

func WithContext(ctx context.Context) Config {
	return opt(func(r *RequestOptions) {
		r.Ctx = ctx
	})
}

func WithLog(log *logrus.Logger) Config {
	return opt(func(r *RequestOptions) {
		r.Log = log
	})
}

func WithLogField(key string, value interface{}) Config {
	return opt(func(r *RequestOptions) {
		r.LogFields[key] = value
	})
}

func WithLogrusFields(fields logrus.Fields) Config {
	return opt(func(r *RequestOptions) {
		for k, v := range fields {
			r.LogFields[k] = v
		}
	})
}

func WithHeader(key string, value string) Config {
	return opt(func(r *RequestOptions) {
		r.Headers[key] = value
	})
}

func WithEndpoint(endpoint string) Config {
	return opt(func(r *RequestOptions) {
		r.Endpoint = endpoint
	})
}

func WithID(id string) Config {
	return opt(func(r *RequestOptions) {
		r.ID = id
	})
}

func WithParams(params interface{}) Config {
	return opt(func(r *RequestOptions) {
		r.Params = params
	})
}

func WithTransientData(key string, val []byte) Config {
	return opt(func(r *RequestOptions) {
		r.Transient[key] = val
	})
}

func WithTransientDataMap(data map[string][]byte) Config {
	return opt(func(r *RequestOptions) {
		for key, val := range data {
			r.Transient[key] = val
		}
	})
}

func WithResponse(target *interface{}) Config {
	return opt(func(r *RequestOptions) {
		r.Target = target
	})
}

func WithAuthToken(token string) Config {
	return opt(func(r *RequestOptions) {
		r.AuthToken = token
	})
}

func WithTimestampGenerator(timestampGenerator func(context.Context) string) Config {
	return opt(func(r *RequestOptions) {
		r.TimestampGenerator = timestampGenerator
	})
}

func WithMSPFilter(mspFilter []string) Config {
	return opt(func(r *RequestOptions) {
		r.MspFilter = append([]string(nil), mspFilter...)
	})
}

func WithMinEndorsers(minEndorsers int) Config {
	return opt(func(r *RequestOptions) {
		r.MinEndorsers = minEndorsers
	})
}

func WithCreator(creator string) Config {
	return opt(func(r *RequestOptions) {
		r.Creator = creator
	})
}

func WithDependentTxID(txID string) Config {
	return opt(func(r *RequestOptions) {
		r.DependentTxID = txID
	})
}

func WithDisableWritePolling(disable bool) Config {
	return opt(func(r *RequestOptions) {
		r.DisableWritePolling = disable
	})
}

func WithCCFetchURLDowngrade(downgrade bool) Config {
	return opt(func(r *RequestOptions) {
		r.CcFetchURLDowngrade = downgrade
	})
}

func WithCCFetchURLProxy(proxy *url.URL) Config {
	return opt(func(r *RequestOptions) {
		r.CcFetchURLProxy = proxy
	})
}

func WithSingleton() Config {
	return opt(func(r *RequestOptions) {})
}
