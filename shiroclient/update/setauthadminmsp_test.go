package update_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient/update"
)

// TestSetAuthAdminMSP pins the request SetAuthAdminMSP sends (the
// $auth_set_admin_msp method, with the MSP as the named parameter "msp")
// and the admin MSP it reads from the answer.  substrate's endpoint is
// new, so the mock plugin this module pins does not have it; the test
// stubs the gateway instead.
func TestSetAuthAdminMSP(t *testing.T) {
	url, got := gatewayStub(t, `{"error_level":0,"result":{"admin_msp":"Org2MSP"},"code":null,"message":null,"data":null}`)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})
	msp, err := update.SetAuthAdminMSP(context.Background(), client, "Org2MSP")
	require.NoError(t, err)
	require.Equal(t, "Org2MSP", msp)

	params := <-got
	require.JSONEq(t, `"$auth_set_admin_msp"`, string(params["method"]))
	require.JSONEq(t, `{"msp":"Org2MSP"}`, string(params["params"]))
}

// TestSetAuthAdminMSPRefused pins that a refusal returns an error holding
// substrate's reason.
func TestSetAuthAdminMSPRefused(t *testing.T) {
	url, _ := gatewayStub(t, `{"error_level":2,"result":null,"code":-32000,"message":"Server error","data":"creator MSP Org2MSP may not set the auth admin MSP"}`)
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint(url)})
	_, err := update.SetAuthAdminMSP(context.Background(), client, "Org2MSP")
	require.EqualError(t, err, `Server error: "creator MSP Org2MSP may not set the auth admin MSP"`)
}

// TestSetAuthAdminMSPEmpty pins that an empty MSP fails before any call,
// as substrate refuses it.
func TestSetAuthAdminMSPEmpty(t *testing.T) {
	client := shiroclient.NewRPC([]shiroclient.Config{shiroclient.WithEndpoint("http://127.0.0.1:0")})
	_, err := update.SetAuthAdminMSP(context.Background(), client, "")
	require.EqualError(t, err, "msp is empty")
}
