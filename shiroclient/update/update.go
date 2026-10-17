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
// use, and a creator outside the admin MSP the phylum set with
// router:set-auth-admin-msp; the error then holds substrate's reason.  The
// chaincode must be a substrate release that has the endpoint.
func SetAuthJWKS(ctx context.Context, client shiroclient.ShiroClient, jwks []byte, configs ...shiroclient.Config) ([]string, error) {
	if !json.Valid(jwks) {
		return nil, errors.New("jwks is not a JSON document")
	}
	configs = append(configs, shiroclient.WithParams(map[string]json.RawMessage{"jwks": jwks}))
	resp, err := client.Call(ctx, setAuthJWKSMethod, configs...)
	if err != nil {
		return nil, err
	}
	if rerr := resp.Error(); rerr != nil {
		if data := rerr.DataJSON(); len(data) > 0 {
			return nil, fmt.Errorf("%s: %s", rerr.Message(), data)
		}
		return nil, errors.New(rerr.Message())
	}
	var out struct {
		KeyIDs []string `json:"key_ids"`
	}
	if err := resp.UnmarshalTo(&out); err != nil {
		return nil, err
	}
	return out.KeyIDs, nil
}
