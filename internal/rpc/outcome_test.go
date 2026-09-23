package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShiroClientOutcome drives the real RPC client against an httptest
// gateway returning both the legacy error shape (data: null) and the
// outcome-unknown shape from luthersystems/substrate#515.
func TestShiroClientOutcome(t *testing.T) {
	methods := []struct {
		name string
		call func(types.ShiroClient) error
	}{
		{"Call", func(c types.ShiroClient) error { _, err := c.Call(context.Background(), "test"); return err }},
		{"Init", func(c types.ShiroClient) error { return c.Init(context.Background(), "phylum") }},
		{"Seed", func(c types.ShiroClient) error { return c.Seed(context.Background(), "version") }},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			var code interface{}
			if method.name == "Call" {
				code = 1
			}
			cases := []struct {
				name    string
				data    string
				code    interface{}
				message interface{}
				unknown bool
				txID    string
			}{
				{"old", `null`, code, "transaction timeout", false, ""},
				{"unknown", `{"outcome":"unknown","tx_id":"tx-123"}`, code, "transaction outcome unknown: txid=tx-123: transaction timeout (reconcile txid=tx-123 before retrying)", true, "tx-123"},
				{"non-timeout", `{"outcome":"unknown","tx_id":"init-123"}`, nil, "transaction outcome unknown", true, "init-123"},
				{"missing tx id", `{"outcome":"unknown"}`, code, "unknown", true, ""},
				{"empty tx id", `{"outcome":"unknown","tx_id":""}`, code, "unknown", true, ""},
				{"unrelated object", `{"foo":"bar"}`, code, "transaction timeout", false, ""},
				{"string", `"unknown"`, code, "transaction timeout", false, ""},
				{"array", `[{"outcome":"unknown"}]`, code, "transaction timeout", false, ""},
				{"other outcome", `{"outcome":"known","tx_id":"tx-123"}`, code, "transaction timeout", false, ""},
				{"non-string outcome", `{"outcome":["unknown"]}`, code, "transaction timeout", false, ""},
				{"old no message", `null`, code, nil, false, ""},
				{"unknown no message", `{"outcome":"unknown","tx_id":"tx-123"}`, code, nil, true, "tx-123"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					body, err := json.Marshal(map[string]interface{}{
						"jsonrpc": "2.0", "id": 1,
						"result": map[string]interface{}{"error_level": 1, "result": nil, "code": tc.code, "message": tc.message, "data": json.RawMessage(tc.data)},
					})
					require.NoError(t, err)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write(body)
					}))
					defer srv.Close()
					client := NewRPC([]types.Config{types.Opt(func(r *types.RequestOptions) { r.Endpoint, r.HTTPClient = srv.URL, srv.Client() })})
					err = method.call(client)
					require.Error(t, err)
					message, ok := tc.message.(string)
					if !ok {
						message = "shiroclient error with no message"
					}
					assert.EqualError(t, err, message)
					assert.Equal(t, tc.code == 1 && ok, IsTimeoutError(err))
					if tc.unknown {
						assert.NotNil(t, errors.Unwrap(err), "outcome-unknown cause must be preserved")
					} else {
						assert.Nil(t, errors.Unwrap(err))
					}
					wrapped := fmt.Errorf("wrapped: %w", err)
					assert.Equal(t, IsTimeoutError(err), IsTimeoutError(wrapped))
					for _, check := range []error{err, wrapped} {
						assert.Equal(t, tc.unknown, errors.Is(check, ErrOutcomeUnknown))
						var outcome *OutcomeUnknownError
						assert.Equal(t, tc.unknown, errors.As(check, &outcome))
						if outcome != nil {
							assert.Equal(t, tc.txID, outcome.TxID)
						}
						txID, ok := OutcomeUnknownTxID(check)
						assert.Equal(t, tc.unknown, ok)
						assert.Equal(t, tc.txID, txID)
					}
				})
			}
		})
	}
}

func TestOutcomeUnknownCause(t *testing.T) {
	res := &rpcres{
		message: "transaction timeout", code: float64(1),
		data: map[string]interface{}{"outcome": "unknown", "tx_id": "tx-123"},
	}
	err := res.getShiroClientError()
	assert.EqualError(t, err, "transaction timeout")
	assert.True(t, IsTimeoutError(err))
	require.NotNil(t, errors.Unwrap(err), "outcome-unknown cause must be preserved")
	assert.Contains(t, errors.Unwrap(err).Error(), "tx-123")
}
