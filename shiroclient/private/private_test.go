package private_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/private"
	"github.com/luthersystems/substratecommon"
	"github.com/stretchr/testify/require"

	_ "embed"
)

//go:embed private_test.lisp
var testPhylum []byte

func newMockClient(conn substratecommon.Substrate) (shiroclient.MockShiroClient, error) {
	client, err := shiroclient.NewMock(nil)
	if err != nil {
		return nil, err
	}

	version, err := client.ShiroPhylum()
	if err != nil {
		return nil, err
	}

	if version != "test" {
		return nil, fmt.Errorf("expected version 'test'")
	}

	err = client.Init(shiroclient.EncodePhylumBytes(testPhylum))
	if err != nil {
		return nil, err
	}

	return client, nil
}

func TestPrivate(t *testing.T) {
	var tests = []struct {
		Name string
		Func func(t *testing.T, client shiroclient.ShiroClient)
	}{
		{
			Name: "export missing",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				data, err := private.Export(context.Background(), client, "DSID-missing")
				if err != nil {
					// NOTE: in future versions this will return an error
					t.Fatalf("unexpected error: %s", err)
				}
				if len(data) != 0 {
					t.Fatalf("expected empty data")
				}
			},
		},
		{
			Name: "purge missing",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				err := private.Purge(context.Background(), client, "DSID-missing")
				if err == nil {
					t.Fatal("expected error")
				}
			},
		},
		{
			Name: "profile to missing DSID",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				dsid, err := private.ProfileToDSID(context.Background(), client, []string{"profile-missing"})
				if err != nil {
					// NOTE: this will return an error in future substrate versions
					t.Fatalf("unexpected error: %s", err)
				}
				if dsid != "" {
					t.Fatal("expected missing DSID")
				}
			},
		},
		{
			Name: "encode zero tranforms",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string
					Fnord string
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				_, err := private.Encode(context.Background(), client, message, transforms)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
			},
		},
		{
			Name: "encode and decode (zero tranforms)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				resp, err := private.Encode(context.Background(), client, message, transforms)
				if err != nil {
					t.Fatalf("encode: %s", err)
				}
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err = private.Decode(context.Background(), client, resp, &decodedMessage)
				if err != nil {
					t.Fatalf("decode: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch, expected: %v != got: %v", message, decodedMessage)
				}
			},
		},
		{
			Name: "encode and decode (1 tranform)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				transforms = append(transforms, &private.Transform{
					ContextPath: ".",
					Header: &private.TransformHeader{
						ProfilePaths: []string{".fnord"},
						PrivatePaths: []string{"."},
						Encryptor:    private.EncryptorAES256,
						Compressor:   private.CompressorZlib,
					},
				})
				resp, err := private.Encode(context.Background(), client, message, transforms)
				if err != nil {
					t.Fatalf("encode: %s", err)
				}
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err = private.Decode(context.Background(), client, resp, &decodedMessage)
				if err != nil {
					t.Fatalf("decode: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch")
				}
			},
		},
		{
			Name: "wrap",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				transforms = append(transforms, &private.Transform{
					ContextPath: ".",
					Header: &private.TransformHeader{
						ProfilePaths: []string{".fnord"},
						PrivatePaths: []string{"."},
						Encryptor:    private.EncryptorAES256,
						Compressor:   private.CompressorZlib,
					},
				})
				wrap := private.WrapCall(context.Background(), client, "wrap_all", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				config, err := private.WithSeed()
				if err != nil {
					t.Fatalf("iv: %s", err)
				}
				err = wrap(message, &decodedMessage, config)
				if err != nil {
					t.Fatalf("wrap: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch: expected: %v != got: %v", message, decodedMessage)
				}
			},
		},
		{
			Name: "no wrap (encode/decode passthrough)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				wrap := private.WrapCall(context.Background(), client, "wrap_none", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err := wrap(message, &decodedMessage)
				if err != nil {
					t.Fatalf("wrap: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch: expected: %v != got: %v", message, decodedMessage)
				}
			},
		},
		{
			// IMPORTANT: this test must run after `wrap`!
			Name: "partial wrap (no encode, yes decode)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				wrap := private.WrapCall(context.Background(), client, "wrap_output", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err := wrap(message, &decodedMessage)
				if err != nil {
					t.Fatalf("wrap: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch: expected: %v != got: %v", message, decodedMessage)
				}
			},
		},
		{
			Name: "partial wrap (yes encode, no decode)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				transforms = append(transforms, &private.Transform{
					ContextPath: ".",
					Header: &private.TransformHeader{
						ProfilePaths: []string{".fnord"},
						PrivatePaths: []string{"."},
						Encryptor:    private.EncryptorAES256,
						Compressor:   private.CompressorZlib,
					},
				})
				wrap := private.WrapCall(context.Background(), client, "wrap_input", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err := wrap(message, &decodedMessage)
				if err != nil {
					t.Fatalf("wrap: %s", err)
				}
				if message != decodedMessage {
					t.Fatalf("message mismatch: expected: %v != got: %v", message, decodedMessage)
				}
			},
		},
		{
			Name: "wrap error (no IV)",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				message := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{
					"world",
					"fnord",
				}
				var transforms []*private.Transform
				transforms = append(transforms, &private.Transform{
					ContextPath: ".",
					Header: &private.TransformHeader{
						ProfilePaths: []string{".fnord"},
						PrivatePaths: []string{"."},
						Encryptor:    private.EncryptorAES256,
						Compressor:   private.CompressorZlib,
					},
				})
				wrap := private.WrapCall(context.Background(), client, "wrap_all", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err := wrap(message, &decodedMessage)
				if err == nil {
					t.Fatalf("expected IV error")
				}
			},
		},
		{
			// IMPORTANT: this test must run after `wrap`!
			// Also: this test must be run if `wrap` is (else build fails)
			Name: "export/purge ok",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				ctx := context.Background()
				dsid, err := private.ProfileToDSID(ctx, client, []string{"fnord"})
				if err != nil {
					t.Fatalf("unexpected profile error: %s", err)
				}

				exportedData, err := private.Export(ctx, client, dsid)
				if err != nil {
					t.Fatalf("unexpected export error: %s", err)
				}

				prettyData, err := json.Marshal(exportedData)
				if err != nil {
					t.Fatalf("unexpected unmarshal error: %s", err)
				}

				if string(prettyData) != `{"test-key":{"fnord":"fnord","hello":"world"}}` {
					t.Fatalf("unexpected data: %s", string(prettyData))
				}

				err = private.Purge(ctx, client, dsid)
				if err != nil {
					t.Fatalf("unexpected purge: error %s", err)
				}

				dsid, err = private.ProfileToDSID(ctx, client, []string{"fnord"})
				if err != nil {
					t.Fatalf("unexpected profile error: %s", err)
				}
				if dsid != "" {
					t.Fatalf("expected empty DSID")
				}
			},
		},
		{
			// IMPORTANT: this test must run after `wrap`!
			Name: "wrap no-op",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				var transforms []*private.Transform
				wrap := private.WrapCall(context.Background(), client, "nop", transforms...)
				decodedMessage := struct{}{}
				err := wrap(nil, &decodedMessage)
				if err != nil {
					t.Fatalf("wrap: %s", err)
				}
				if struct{}{} != decodedMessage {
					t.Fatalf("message mismatch: expected: %v != got: %v", nil, decodedMessage)
				}
			},
		},
	}
	err := substratecommon.Connect((func(conn substratecommon.Substrate) error {
		client, err := newMockClient(conn)
		if err != nil {
			t.Fatalf("mock client: %s", err)
		}
		defer func() {
			err := client.Close()
			require.NoError(t, err)
		}()
		for _, tc := range tests {
			t.Run(tc.Name, func(t *testing.T) {
				tc.Func(t, client)
			})
		}

		return nil
	}),
		substratecommon.ConnectWithCommand(os.Getenv("SUBSTRATEHCP_FILE")),
		substratecommon.ConnectWithAttachStdamp(os.Stderr))
	require.Nil(t, err)
}
