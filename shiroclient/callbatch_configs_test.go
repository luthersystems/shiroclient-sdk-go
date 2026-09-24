package shiroclient_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
)

// batchRequests returns the "requests" parameter of a recorded CallBatch.
func batchRequests(t *testing.T, got <-chan map[string]interface{}) ([]interface{}, map[string]interface{}) {
	t.Helper()
	params := (<-got)["params"].(map[string]interface{})
	return params["requests"].([]interface{}), params
}

func TestCallBatchRequestConfigsTransient(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "deposit", Configs: []shiroclient.Config{
			shiroclient.WithTransientData("secret", []byte("alice")),
		}},
		{Method: "deposit", Configs: []shiroclient.Config{
			shiroclient.WithTransientDataMap(map[string][]byte{"secret": []byte("bob"), "note": []byte("n")}),
		}},
	})
	require.NoError(t, err)
	reqs, params := batchRequests(t, got)
	assert.Equal(t, map[string]interface{}{"secret": hex.EncodeToString([]byte("alice"))},
		reqs[0].(map[string]interface{})["transient"])
	assert.Equal(t, map[string]interface{}{
		"secret": hex.EncodeToString([]byte("bob")),
		"note":   hex.EncodeToString([]byte("n")),
	}, reqs[1].(map[string]interface{})["transient"], "each request carries only its own transient data")
	assert.Equal(t, map[string]interface{}{}, params["transient"], "nothing is shared")
}

func TestCallBatchRequestConfigsLaterWins(t *testing.T) {
	client, got, _ := batchGateway(t, batchCommitted)
	_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
		{Method: "a", Configs: []shiroclient.Config{
			shiroclient.WithTransientData("k", []byte("first")),
			shiroclient.WithTransientData("k", []byte("second")),
		}},
		{Method: "b", Configs: []shiroclient.Config{shiroclient.WithSingleton()}},
	})
	require.NoError(t, err)
	reqs, _ := batchRequests(t, got)
	assert.Equal(t, map[string]interface{}{"k": hex.EncodeToString([]byte("second"))},
		reqs[0].(map[string]interface{})["transient"], "configs apply in order, as for Call")
	assert.NotContains(t, reqs[1].(map[string]interface{}), "transient", "a no-op config sends no transient field")
}

// TestCallBatchRequestConfigsRefused pins every Call config that cannot apply
// to one request of a single transaction: each is refused before anything is
// sent.
func TestCallBatchRequestConfigsRefused(t *testing.T) {
	client, _, hits := batchGateway(t, batchCommitted)
	proxy, err := url.Parse("http://proxy.example")
	require.NoError(t, err)
	var target interface{}
	cases := map[string]shiroclient.Config{
		"WithParams":              shiroclient.WithParams([]interface{}{1}),
		"WithID":                  shiroclient.WithID("x"),
		"WithResponse":            shiroclient.WithResponse(&target),
		"WithLog":                 shiroclient.WithLog(logrus.New()),
		"WithLogField":            shiroclient.WithLogField("k", "v"),
		"WithLogrusFields":        shiroclient.WithLogrusFields(logrus.Fields{"k": "v"}),
		"WithHeader":              shiroclient.WithHeader("X-K", "v"),
		"WithCCFetchURLProxy":     shiroclient.WithCCFetchURLProxy(proxy),
		"WithHTTPClient":          shiroclient.WithHTTPClient(http.DefaultClient),
		"WithTimestampGenerator":  shiroclient.WithTimestampGenerator(func(context.Context) string { return "t" }),
		"WithEndpoint":            shiroclient.WithEndpoint("http://other"),
		"WithPhylumVersion":       shiroclient.WithPhylumVersion("v2"),
		"WithDependentBlock":      shiroclient.WithDependentBlock("5"),
		"WithAuthToken":           shiroclient.WithAuthToken("tok"),
		"WithCreator":             shiroclient.WithCreator("Org2MSP"),
		"WithDependentTxID":       shiroclient.WithDependentTxID("tx"),
		"WithoutTargetEndpoints":  shiroclient.WithoutTargetEndpoints([]string{"peer1"}),
		"WithTargetEndpoints":     shiroclient.WithTargetEndpoints([]string{"peer0"}),
		"WithMSPFilter":           shiroclient.WithMSPFilter([]string{"Org1MSP"}),
		"WithMinEndorsers":        shiroclient.WithMinEndorsers(2),
		"WithDisableWritePolling": shiroclient.WithDisableWritePolling(true),
		"WithCCFetchURLDowngrade": shiroclient.WithCCFetchURLDowngrade(true),
		"WithResponseReceiver":    shiroclient.WithResponseReceiver(func(shiroclient.ShiroResponse) {}),
		"WithUnsafeDebug":         shiroclient.WithUnsafeDebug(),
		"nil config":              nil,
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := shiroclient.CallBatch(context.Background(), client, []shiroclient.CallBatchRequest{
				{Method: "a"},
				{Method: "b", Configs: []shiroclient.Config{
					shiroclient.WithTransientData("ok", []byte("x")),
					cfg,
				}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "request 1")
			if name != "nil config" {
				assert.Contains(t, err.Error(), name, "the error names the refused option")
			}
		})
	}
	assert.Equal(t, int32(0), atomic.LoadInt32(hits), "a refused config sends nothing")
}
