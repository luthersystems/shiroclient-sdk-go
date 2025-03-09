// Package update manages phylum versions installed on substrate.
package update

import (
	"context"
	"errors"

	"github.com/luthersystems/shiroclient-sdk-go/internal/types"
	"github.com/luthersystems/shiroclient-sdk-go/shiroclient"
)

const (
	getPhylaMethod = "get_phyla"
	updateMethod   = "update"
	enableMethod   = "enable"
	disableMethod  = "disable"
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
func Install(ctx context.Context, client shiroclient.ShiroClient, version string, phylum []byte, clientConfigs ...shiroclient.Config) error {
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
