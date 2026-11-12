package mock

import (
	"bytes"
	"context"
	"encoding/gob"
	"net/url"
	"testing"
	"time"

	"github.com/luthersystems/shiroclient-sdk-go/internal/mockint"
	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/x/plugin"
	"github.com/stretchr/testify/require"
)

func withProxy(u *url.URL) types.Config {
	return types.Opt(func(r *types.RequestOptions) { r.CcFetchURLProxy = u })
}

func withDowngrade(d bool) types.Config {
	return types.Opt(func(r *types.RequestOptions) { r.CcFetchURLDowngrade = d })
}

// TestMockOptionsFetchURL pins luthersystems/substrate#1084: NewMock sends
// the cc-fetchurl settings of its base configs to the plugin, and each
// request carries the same proxy text, which a substrate mock compares
// with its own.
func TestMockOptionsFetchURL(t *testing.T) {
	proxy, err := url.Parse("http://proxy.example.com:3128")
	require.NoError(t, err)
	base := []types.Config{withProxy(proxy), withDowngrade(true)}
	cfg := &mockint.Config{PreheatTimeout: 5 * time.Second}

	got := mockOptions(cfg, base)
	require.Equal(t, plugin.MockOptions{
		PreheatTimeout:      5 * time.Second,
		CCFetchURLProxy:     "http://proxy.example.com:3128",
		CCFetchURLDowngrade: true,
	}, got)

	c := &mockShiroClient{baseConfig: base}
	cro, err := c.flatten(context.Background())
	require.NoError(t, err)
	require.Equal(t, got.CCFetchURLProxy, cro.CCFetchURLProxy)
	require.Equal(t, got.CCFetchURLDowngrade, cro.CCFetchURLDowngrade)

	require.Equal(t, plugin.MockOptions{}, mockOptions(&mockint.Config{}, nil))
}

// TestMockOptionsGobCompatible pins that MockOptions crosses the plugin RPC
// (gob) both ways between this SDK and a substrate built with the older
// struct, which had only PreheatTimeout. gob matches fields by name and
// drops the ones the other side lacks.
func TestMockOptionsGobCompatible(t *testing.T) {
	type oldMockOptions struct{ PreheatTimeout time.Duration }

	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(plugin.MockOptions{
		PreheatTimeout: time.Second, CCFetchURLProxy: "http://p:1", CCFetchURLDowngrade: true,
	}))
	var old oldMockOptions
	require.NoError(t, gob.NewDecoder(&buf).Decode(&old))
	require.Equal(t, time.Second, old.PreheatTimeout)

	buf.Reset()
	require.NoError(t, gob.NewEncoder(&buf).Encode(oldMockOptions{PreheatTimeout: time.Second}))
	var cur plugin.MockOptions
	require.NoError(t, gob.NewDecoder(&buf).Decode(&cur))
	require.Equal(t, plugin.MockOptions{PreheatTimeout: time.Second}, cur)
}
