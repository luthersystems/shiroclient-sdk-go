// This package includes helpers for the plugin and will be removed in later
// versions.
package mock_deprecated

import (
	"context"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/internal/types"
	"github.com/sirupsen/logrus"
)

type Config = types.Config

func ProbeForCall(configs []Config) (context.Context, string, func(context.Context) string, logrus.Fields, string, string, interface{}, map[string][]byte, error) {
	ro := types.ApplyConfigs(context.TODO(), nil, configs...)
	return ro.Ctx, ro.ID, ro.TimestampGenerator, ro.LogFields, ro.AuthToken, ro.Creator, ro.Params, ro.Transient, nil
}

func ProbeForNew(configs []Config) (bool, *url.URL, error) {
	ro := types.ApplyConfigs(context.TODO(), nil, configs...)
	return ro.CcFetchURLDowngrade, ro.CcFetchURLProxy, nil
}
