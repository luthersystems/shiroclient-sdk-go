package phylum_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/phylum"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallOutcome runs the phylum client against an httptest gateway: legacy
// timeouts must be byte-for-byte unchanged, while outcome-unknown timeouts keep
// the same gRPC code and message but carry the tx id.
func TestCallOutcome(t *testing.T) {
	const legacy = "rpc error: code = Unavailable desc = timeout in blockchain network"
	const serverMessage = "transaction outcome unknown: txid=tx-123: transaction timeout (reconcile txid=tx-123 before retrying)"
	for _, tc := range []struct {
		name    string
		data    string
		code    interface{}
		unknown bool
		txID    string
	}{
		{"new timeout", `{"outcome":"unknown","tx_id":"tx-123"}`, 1, true, "tx-123"},
		{"old timeout", `null`, 1, false, ""},
		{"unrelated data", `{"foo":"bar"}`, 1, false, ""},
		{"missing tx id", `{"outcome":"unknown"}`, 1, true, ""},
		{"non-timeout", `{"outcome":"unknown","tx_id":"tx-123"}`, nil, true, "tx-123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]interface{}{"error_level": 1, "result": nil, "code": tc.code, "message": serverMessage, "data": json.RawMessage(tc.data)},
			})
			require.NoError(t, err)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
			defer srv.Close()
			config := shiroclient.WithHTTPClient(srv.Client())
			client, err := phylum.New(srv.URL, logrus.NewEntry(logrus.New()))
			require.NoError(t, err)
			for _, call := range []struct {
				name   string
				invoke func() error
				prefix string
			}{
				{"Call", func() error {
					_, err := phylum.Call(client, context.Background(), "test", wrapperspb.String("request"), &wrapperspb.StringValue{}, config)
					return err
				}, ""},
				{"GetAppControlProperty", func() error { _, err := client.GetAppControlProperty(context.Background(), "test", config); return err }, `failed to get app control property "test": `},
				{"SetAppControlProperty", func() error { return client.SetAppControlProperty(context.Background(), "test", "value", config) }, `failed to set app control property "test": `},
			} {
				t.Run(call.name, func(t *testing.T) {
					err := call.invoke()
					require.Error(t, err)
					for _, check := range []error{err, fmt.Errorf("wrapped: %w", err)} {
						txID, ok := phylum.AmbiguousTxID(check)
						assert.Equal(t, tc.unknown, ok)
						assert.Equal(t, tc.txID, txID)
						txID, ok = shiroclient.OutcomeUnknownTxID(check)
						assert.Equal(t, tc.unknown, ok)
						assert.Equal(t, tc.txID, txID)
						assert.Equal(t, tc.unknown, errors.Is(check, shiroclient.ErrOutcomeUnknown))
						var outcome *shiroclient.OutcomeUnknownError
						assert.Equal(t, tc.unknown, errors.As(check, &outcome))
						if outcome != nil {
							assert.Equal(t, tc.txID, outcome.TxID)
						}
					}
					if tc.code == nil {
						assert.EqualError(t, err, call.prefix+serverMessage)
						return
					}
					assert.EqualError(t, err, call.prefix+legacy)
					assert.Equal(t, codes.Unavailable, status.Code(err))
					st, ok := status.FromError(err)
					require.True(t, ok)
					if !tc.unknown {
						assert.Empty(t, st.Details())
						return
					}
					require.Len(t, st.Details(), 1)
					info, ok := st.Details()[0].(*errdetails.ErrorInfo)
					require.True(t, ok)
					assert.Equal(t, "TX_OUTCOME_UNKNOWN", phylum.ReasonTxOutcomeUnknown)
					assert.Equal(t, phylum.ReasonTxOutcomeUnknown, info.Reason)
					assert.Equal(t, "shiroclient.luthersystems.com", info.Domain)
					assert.Equal(t, map[string]string{"tx_id": tc.txID}, info.Metadata)
					for _, remote := range []error{st.Err(), fmt.Errorf("wrapped: %w", st.Err())} {
						txID, ok := phylum.AmbiguousTxID(remote)
						assert.True(t, ok)
						assert.Equal(t, tc.txID, txID)
					}
					wrappedStatus, ok := status.FromError(fmt.Errorf("wrapped: %w", st.Err()))
					require.True(t, ok)
					assert.Equal(t, st.Details(), wrappedStatus.Details())
				})
			}
		})
	}
}

func TestAmbiguousTxID(t *testing.T) {
	for _, tc := range []struct {
		name string
		info *errdetails.ErrorInfo
		ok   bool
	}{
		{"unknown", &errdetails.ErrorInfo{Reason: "TX_OUTCOME_UNKNOWN", Domain: "shiroclient.luthersystems.com", Metadata: map[string]string{"tx_id": "tx-123"}}, true},
		{"missing id", &errdetails.ErrorInfo{Reason: "TX_OUTCOME_UNKNOWN", Domain: "shiroclient.luthersystems.com"}, true},
		{"different reason", &errdetails.ErrorInfo{Reason: "OTHER", Domain: "shiroclient.luthersystems.com"}, false},
		{"different domain", &errdetails.ErrorInfo{Reason: "TX_OUTCOME_UNKNOWN", Domain: "other.example"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := status.New(codes.Unavailable, "timeout").WithDetails(tc.info)
			require.NoError(t, err)
			for _, err := range []error{st.Err(), fmt.Errorf("wrapped: %w", st.Err())} {
				txID, ok := phylum.AmbiguousTxID(err)
				assert.Equal(t, tc.ok, ok)
				assert.Equal(t, tc.info.Metadata["tx_id"], txID)
			}
		})
	}
	for _, err := range []error{nil, errors.New("unknown"), status.Error(codes.Unavailable, "timeout")} {
		txID, ok := phylum.AmbiguousTxID(err)
		assert.False(t, ok)
		assert.Empty(t, txID)
	}
}
