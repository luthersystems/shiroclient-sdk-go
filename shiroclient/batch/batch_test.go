package batch_test

import (
	"testing"

	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/batch"

	_ "embed"
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

	ticker := driver.Register(context.Background(), "test_batch", time.Duration(1)*time.Hour, func(batchID string, requestID string, message json.RawMessage) (json.RawMessage, error) {
		/****/ if string(message) == "\"ping1\"" {
			lastReceivedMessage = "ping1"
			return []byte("\"pong1\""), nil
		} else if string(message) == "\"ping2\"" {
			lastReceivedMessage = "ping2"
			return nil, errors.New("ping2 error")
		} else if string(message) == "\"ping3\"" {
			lastReceivedMessage = "ping3"
			return []byte("\"pong3\""), nil
		} else {
			panic(nil)
		}
	})

	recentInput := ""

	doTick := func(t *testing.T) {
		ticker.Tick(context.Background())

		sr, err := client.Call(context.Background(), "get_recent_input", shiroclient.WithParams([]interface{}{}))
		if err != nil || sr.Error() != nil {
			t.Fatal()
		}

		err = json.Unmarshal(sr.ResultJSON(), &recentInput)
		if err != nil {
			t.Fatal(err)
		}
	}

	table := []struct {
		name      string
		method    string
		params    interface{}
		validator func(*testing.T) bool
	}{
		{
			"first test - immediately scheduled batch request",
			"schedule_request_now",
			[]interface{}{
				"test_batch",
				"ping1",
			},
			func(t *testing.T) bool {
				return lastReceivedMessage == "ping1" && recentInput == "pong1"
			},
		},

		{
			"second test - immediately scheduled batch request with failure",
			"schedule_request_now",
			[]interface{}{
				"test_batch",
				"ping2",
			},
			func(t *testing.T) bool {
				return lastReceivedMessage == "ping2" && recentInput == "error: ping2 error"
			},
		},

		{
			"third test - schedule at times other than now",
			"schedule_request",
			[]interface{}{
				"test_batch",
				"ping3",
				TS002,
			},
			func(t *testing.T) bool {
				// should not have got ping3 yet
				if lastReceivedMessage != "ping2" {
					return false
				}

				// now artificially advance time
				tsAssign(TS003)

				// tick (again)
				doTick(t)

				// now it should have worked
				return lastReceivedMessage == "ping3" && recentInput == "pong3"
			},
		},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			sr, err := client.Call(context.Background(), tt.method, shiroclient.WithParams(tt.params))
			if err != nil || sr.Error() != nil {
				t.Fatal()
			}

			doTick(t)

			if !(tt.validator(t)) {
				t.Fatal()
			}
		})
	}

}
