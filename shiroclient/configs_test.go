package shiroclient_test

import (
	"encoding/json"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestMarshalUnmarshalWithDiscardUnknown(t *testing.T) {
	// 1) Create a minimal Any and marshal to JSON object
	orig := &anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.Empty", Value: nil}
	jsonBytes, err := protojson.Marshal(orig)
	assert.NoError(t, err)

	// 2) Inject an extra field via a map round-trip
	var m map[string]interface{}
	assert.NoError(t, json.Unmarshal(jsonBytes, &m))
	m["mystery"] = "surprise"
	payload, err := json.Marshal(m)
	assert.NoError(t, err)

	// remember and restore global flag
	origFlag := types.UnmarshalOptions.DiscardUnknown
	defer shiroclient.SetDiscardUnknownFields(origFlag)

	tests := []struct {
		name           string
		discardUnknown bool
		wantErr        bool
	}{
		{"error on unknown when discard=false", false, true},
		{"ignore unknown when discard=true", true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			shiroclient.SetDiscardUnknownFields(tc.discardUnknown)

			got := &anypb.Any{}
			err := types.UnmarshalProto(payload, got)

			if tc.wantErr {
				assert.Error(t, err, "expected an error for mystery field")
			} else {
				assert.NoError(t, err, "did not expect error when discarding unknowns")
				assert.Equal(t, orig.GetTypeUrl(), got.GetTypeUrl(), "TypeUrl round-trips")
			}
		})
	}
}
