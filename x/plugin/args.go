// Package plugin includes helpers for the substrate plugin implementation
// to extract configuration arguments.
// WARNING: This is unstable will be removed in later versions.
package plugin

import (
	"context"
	"net/url"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/sirupsen/logrus"
)

type pluginArgs struct {
	ro *types.RequestOptions
}

func PluginArgs(configs []types.Config) pluginArgs {
	return pluginArgs{ro: types.ApplyConfigs(context.TODO(), nil, configs...)}
}

func PluginCtx(p pluginArgs) context.Context {
	return p.ro.Ctx
}

func PluginID(p pluginArgs) string {
	return p.ro.ID
}

func PluginTimestampGenerator(p pluginArgs) func(context.Context) string {
	return p.ro.TimestampGenerator
}

func PluginLogFields(p pluginArgs) logrus.Fields {
	return p.ro.LogFields
}

func PluginAuthToken(p pluginArgs) string {
	return p.ro.AuthToken
}

func PluginCreator(p pluginArgs) string {
	return p.ro.Creator
}

func PluginParams(p pluginArgs) interface{} {
	return p.ro.Params
}

func PluginTransient(p pluginArgs) map[string][]byte {
	return p.ro.Transient
}

func PluginCcFetchURLDowngrade(p pluginArgs) bool {
	return p.ro.CcFetchURLDowngrade
}

func PluginCcFetchURLProxy(p pluginArgs) *url.URL {
	return p.ro.CcFetchURLProxy
}

func PluginPhylumVersion(p pluginArgs) string {
	return p.ro.PhylumVersion
}
