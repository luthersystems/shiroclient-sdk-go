package batch_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/batch"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed batch_test.lisp
var testPhylum []byte

func Test001(t *testing.T) {
	var TS001 = "2000-01-01T00:00:00-08:00"
	var TS002 = "2000-01-02T00:00:00-08:00"
	var TS003 = "2000-01-03T00:00:00-08:00"

	var tsMutex *sync.Mutex
	var tsString string

	tsMutex = &sync.Mutex{}
	tsString = TS001

	tsAssign := func(tsInput string) {
		tsMutex.Lock()
		defer tsMutex.Unlock()

		tsString = tsInput
	}

	tsGenerator := func(ctx context.Context) string {
		tsMutex.Lock()
		defer tsMutex.Unlock()

		return tsString
	}

	log := logrus.New()

	log.SetLevel(logrus.DebugLevel)

	clientConfigs := []shiroclient.Config{
		shiroclient.WithLog(log),
		shiroclient.WithTimestampGenerator(tsGenerator),
	}
	client, err := shiroclient.NewMock(clientConfigs)
	require.Nil(t, err)
	defer func() {
		err := client.Close()
		require.NoError(t, err)
	}()

	err = client.Init(shiroclient.EncodePhylumBytes(testPhylum))
	if err != nil {
		t.Fatal(err)
	}

	driver := batch.NewDriver(client, batch.WithLog(log), batch.WithLogField("TESTFIELD", "TESTVALUE"))

	lastReceivedMessage := "none"

	ctx := context.Background()

	ticker := driver.Register(ctx, "test_batch", time.Duration(1)*time.Hour, func(batchID string, requestID string, message json.RawMessage) (json.RawMessage, error) {
		messageStr := string(message)
		switch messageStr {
		case `"ping1"`:
			lastReceivedMessage = "ping1"
			return []byte("\"pong1\""), nil
		case `"ping2"`:
			lastReceivedMessage = "ping2"
			return nil, errors.New("ping2 error")
		case `"ping3"`:
			lastReceivedMessage = "ping3"
			return []byte("\"pong3\""), nil
		default:
			panic(nil)
		}
	})

	recentInput := ""

	doTick := func(t *testing.T) {
		ticker.Tick(ctx)

		sr, err := client.Call(ctx, "get_recent_input", shiroclient.WithParams([]interface{}{}))
		require.NoError(t, err, "Error calling get_recent_input")
		require.Nil(t, sr.Error(), "Result contains an error")

		err = json.Unmarshal(sr.ResultJSON(), &recentInput)
		require.NoError(t, err, "Error unmarshalling JSON")
	}

	table := []struct {
		name      string
		method    string
		params    interface{}
		validator func(*testing.T)
	}{
		{
			"first test - immediately scheduled batch request",
			"schedule_request_now",
			[]interface{}{
				"test_batch",
				"ping1",
			},
			func(t *testing.T) {
				assert.Equal(t, "ping1", lastReceivedMessage, "lastReceivedMessage should be 'ping1'")
				assert.Equal(t, "pong1", recentInput, "recentInput should be 'pong1'")
			},
		},

		{
			"second test - immediately scheduled batch request with failure",
			"schedule_request_now",
			[]interface{}{
				"test_batch",
				"ping2",
			},
			func(t *testing.T) {
				assert.Equal(t, "ping2", lastReceivedMessage, "lastReceivedMessage should be 'ping2'")
				assert.Equal(t, "error: ping2 error", recentInput, "recentInput should be 'error: ping2 error'")
			},
		},

		// TODO: fix this test
		{
			"third test - schedule at times other than now",
			"schedule_request",
			[]interface{}{
				"test_batch",
				"ping3",
				TS002,
			},
			func(t *testing.T) {
				// First check: lastReceivedMessage should be "ping2". If not, the test should fail and stop.
				require.Equal(t, "ping2", lastReceivedMessage, "Expected lastReceivedMessage to be 'ping2' before advancing time")

				// Now artificially advance time
				tsAssign(TS003)

				// Tick (again)
				doTick(t)

				// Final checks: lastReceivedMessage should be "ping3" and recentInput should be "pong3".
				// These are more assert checks as they are testing the outcome after the tick.
				assert.Equal(t, "ping3", lastReceivedMessage, "Expected lastReceivedMessage to be 'ping3' after ticking")
				assert.Equal(t, "pong3", recentInput, "Expected recentInput to be 'pong3' after ticking")
			},
		},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			sr, err := client.Call(ctx, tt.method, shiroclient.WithParams(tt.params))
			require.NoError(t, err)
			require.NoError(t, sr.Error())

			doTick(t)

			tt.validator(t)
		})
	}

}
