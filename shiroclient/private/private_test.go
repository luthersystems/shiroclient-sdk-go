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
				require.NoError(t, err)
				require.Empty(t, data)
			},
		},
		{
			Name: "purge missing",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				err := private.Purge(context.Background(), client, "DSID-missing")
				require.Error(t, err)
			},
		},
		{
			Name: "profile to missing DSID",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				dsid, err := private.ProfileToDSID(context.Background(), client, []string{"profile-missing"})
				require.NoError(t, err)
				require.Empty(t, dsid)
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
				require.NoError(t, err)
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
				require.NoError(t, err)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err = private.Decode(context.Background(), client, resp, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, decodedMessage, message)
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
				require.NoError(t, err)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				err = private.Decode(context.Background(), client, resp, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, decodedMessage, message)
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
				wrap := private.WrapCall(client, "wrap_all", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				config, err := private.WithSeed()
				require.NoError(t, err)
				cr, err := wrap(context.Background(), message, &decodedMessage, config)
				require.NoError(t, err)
				require.NotEmpty(t, cr.TransactionID)
				require.Equal(t, decodedMessage, message)
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
				wrap := private.WrapCall(client, "wrap_none", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				_, err := wrap(context.Background(), message, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, decodedMessage, message)
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
				wrap := private.WrapCall(client, "wrap_output", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				_, err := wrap(context.Background(), message, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, decodedMessage, message)
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
				wrap := private.WrapCall(client, "wrap_input", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				_, err := wrap(context.Background(), message, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, decodedMessage, message)
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
				wrap := private.WrapCall(client, "wrap_all", transforms...)
				decodedMessage := struct {
					Hello string `json:"hello"`
					Fnord string `json:"fnord"`
				}{}
				_, err := wrap(context.Background(), message, &decodedMessage)
				require.Error(t, err)
			},
		},
		{
			// IMPORTANT: this test must run after `wrap`!
			// Also: this test must be run if `wrap` is (else build fails)
			Name: "export/purge ok",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				ctx := context.Background()
				dsid, err := private.ProfileToDSID(ctx, client, []string{"fnord"})
				require.NoError(t, err)
				exportedData, err := private.Export(ctx, client, dsid)
				require.NoError(t, err)
				prettyData, err := json.Marshal(exportedData)
				require.NoError(t, err)
				expected := `{"test-key":{"fnord":"fnord","hello":"world"}}`
				require.Equal(t, expected, string(prettyData))
				err = private.Purge(ctx, client, dsid)
				require.NoError(t, err)
				dsid, err = private.ProfileToDSID(ctx, client, []string{"fnord"})
				require.NoError(t, err)
				require.Empty(t, dsid)
			},
		},
		{
			// IMPORTANT: this test must run after `wrap`!
			Name: "wrap no-op",
			Func: func(t *testing.T, client shiroclient.ShiroClient) {
				var transforms []*private.Transform
				wrap := private.WrapCall(client, "nop", transforms...)
				decodedMessage := struct{}{}
				_, err := wrap(context.Background(), nil, &decodedMessage)
				require.NoError(t, err)
				require.Equal(t, struct{}{}, decodedMessage)
			},
		},
	}
	err := substratecommon.Connect((func(conn substratecommon.Substrate) error {
		client, err := newMockClient(conn)
		require.NoError(t, err)
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
