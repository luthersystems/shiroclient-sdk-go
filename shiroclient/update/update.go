// Package update manages phylum versions installed on substrate.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
)

const (
	getPhylaMethod = "get_phyla"
	updateMethod   = "update"
	enableMethod   = "enable"
	disableMethod  = "disable"
	// setAuthJWKSMethod is substrate's endpoint that writes the ledger
	// JWKS (luthersystems/substrate#676).
	setAuthJWKSMethod = "$auth_set_jwks"
	// setAuthAdminMSPMethod is substrate's endpoint that replaces the
	// admin MSP of the auth methods (luthersystems/substrate#676).
	setAuthAdminMSPMethod = "$auth_set_admin_msp"
)

const (
	// LatestPhylumVersion selects the latest version of the phylum.
	LatestPhylumVersion = "latest"
)

// StatusType is a custom type to represent the status of a Phylum.
type StatusType string

const (
	// StatusInService indicates the phylum is installed and enabled.
	StatusInService StatusType = "IN_SERVICE"
	// StatusDisabled indicates the phylum is installed and disabled.
	StatusDisabled StatusType = "DISABLED"
)

// Phlya lists installed phylum.
type Phyla struct {
	// Phyla is the settings for all installed phyla.
	Phyla []*PhylumSettings `json:"phyla"`
}

// PhylumSettings are the settings for a phylum.
type PhylumSettings struct {
	// Fingerprint is a checksum of the code.
	Fingerprint string `json:"fingerprint"`
	// InitTimestamp is the RFC3339 time the code was installed.
	InitTimestamp string `json:"init_timestamp"`
	// PhylumID is an ID for the phylum.
	PhylumID string `json:"phylum_id"`
	// Status is a StatusType.
	Status StatusType `json:"status"`
}

// GetPhyla returns installed phyla.
func GetPhyla(ctx context.Context, client shiroclient.ShiroClient, configs ...shiroclient.Config) (*Phyla, error) {
	configs = append(configs, shiroclient.WithParams([]string{""}))
	resp, err := client.Call(ctx, getPhylaMethod, configs...)
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, errors.New(resp.Error().Message())
	}

	phyla := &Phyla{}
	err = resp.UnmarshalTo(phyla)
	if err != nil {
		return nil, err
	}

	return phyla, nil
}

// Enable enables an installed phylum.
func Enable(ctx context.Context, client shiroclient.ShiroClient, version string, configs ...shiroclient.Config) error {
	configs = append(configs, shiroclient.WithParams([]string{version}))
	resp, err := client.Call(ctx, enableMethod, configs...)
	if err != nil {
		return err
	}
	if resp.Error() != nil {
		return errors.New(resp.Error().Message())
	}
	return nil
}

// Disable disables an installed phylum.
func Disable(ctx context.Context, client shiroclient.ShiroClient, version string, configs ...shiroclient.Config) error {
	configs = append(configs, shiroclient.WithParams([]string{version}))
	resp, err := client.Call(ctx, disableMethod, configs...)
	if err != nil {
		return err
	}
	if resp.Error() != nil {
		return errors.New(resp.Error().Message())
	}
	return nil
}

// withNewPhylumVersion sets the version for a newly installed phylum.
func withNewPhylumVersion(newPhylumVersion string) types.Config {
	return types.Opt(func(r *types.RequestOptions) {
		r.NewPhylumVersion = newPhylumVersion
	})
}

// Install adds new phylum to substrate.
func Install(ctx context.Context, client shiroclient.ShiroClient, version string, phylum []byte, clientConfigs ...shiroclient.Config) error { //nolint:revive // public API: changing the signature breaks callers.
	newConfigs := []shiroclient.Config{shiroclient.WithParams([]string{shiroclient.EncodePhylumBytes(phylum)}), withNewPhylumVersion(version)}
	configs := make([]shiroclient.Config, 0, len(newConfigs)+len(clientConfigs))
	configs = append(configs, newConfigs...)
	configs = append(configs, clientConfigs...)
	resp, err := client.Call(ctx, updateMethod, configs...)
	if err != nil {
		return err
	}
	if resp.Error() != nil {
		return errors.New(resp.Error().Message())
	}
	return nil
}

// SetAuthJWKS writes jwks, a JWKS JSON document (RFC 7517,
// {"keys":[...]}), to the ledger as the key set substrate verifies auth
// tokens with when the chaincode runs with
// SUBSTRATE_AUTH_CLAIMS_JWKS_SOURCE=ledger (luthersystems/substrate#676).
// It returns the key IDs the set holds.  Substrate refuses a set it cannot
// use, and a creator outside the admin MSP (the app control property
// auth_jwks_admin_msp, which chaincode Init's member $auth_admin_msp sets
// and SetAuthAdminMSP replaces); with no admin MSP it refuses every
// creator.  The error then holds substrate's reason.  The chaincode must
// be a substrate release that has the endpoint.
func SetAuthJWKS(ctx context.Context, client shiroclient.ShiroClient, jwks []byte, configs ...shiroclient.Config) ([]string, error) {
	if !json.Valid(jwks) {
		return nil, errors.New("jwks is not a JSON document")
	}
	var out struct {
		KeyIDs []string `json:"key_ids"`
	}
	call := authCall{method: setAuthJWKSMethod, params: map[string]json.RawMessage{"jwks": jwks}, out: &out}
	if err := call.run(ctx, client, configs); err != nil {
		return nil, err
	}
	return out.KeyIDs, nil
}

// SetAuthAdminMSP replaces the admin MSP, whose creators may call
// SetAuthJWKS and SetAuthAdminMSP, with msp, and returns the admin MSP
// substrate wrote (luthersystems/substrate#676).  Only a creator of the
// current admin MSP may replace it.  Substrate refuses a ledger with no
// admin MSP: chaincode Init's member $auth_admin_msp sets the first one,
// and the Fabric Init transaction that carries it is sent outside this
// SDK.  Read the admin MSP with the app control property
// auth_jwks_admin_msp (phylum.Client.GetAppControlProperty).  A refusal
// returns an error that holds substrate's reason.  The chaincode must be a
// substrate release that has the endpoint.
func SetAuthAdminMSP(ctx context.Context, client shiroclient.ShiroClient, msp string, configs ...shiroclient.Config) (string, error) {
	if msp == "" {
		return "", errors.New("msp is empty")
	}
	var out struct {
		AdminMSP string `json:"admin_msp"`
	}
	call := authCall{method: setAuthAdminMSPMethod, params: map[string]string{"msp": msp}, out: &out}
	if err := call.run(ctx, client, configs); err != nil {
		return "", err
	}
	return out.AdminMSP, nil
}

// authCall is a call of an auth method: its named params, and where to
// decode its answer.
type authCall struct {
	params any
	out    any
	method string
}

// run calls the auth method and decodes its answer into c.out.  A refusal
// is an error that holds substrate's reason, the error's data.
func (c authCall) run(ctx context.Context, client shiroclient.ShiroClient, configs []shiroclient.Config) error {
	configs = append(configs, shiroclient.WithParams(c.params))
	resp, err := client.Call(ctx, c.method, configs...)
	if err != nil {
		return err
	}
	if rerr := resp.Error(); rerr != nil {
		if data := rerr.DataJSON(); len(data) > 0 {
			return fmt.Errorf("%s: %s", rerr.Message(), data)
		}
		return errors.New(rerr.Message())
	}
	return resp.UnmarshalTo(c.out)
}
