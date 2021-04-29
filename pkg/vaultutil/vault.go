package vaultutil

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	vault "github.com/mittwald/vaultgo"
	"github.com/pkg/errors"
)

type VaultProvider struct {
	Engine string
	Role   string

	VaultClient *vault.Client
	credentials.Expiry
}

type awsResponse struct {
	LeaseDuration int `json:"lease_duration"`
	Data          struct {
		AccessKeyID     string `json:"access_key"`
		SecretAccessKey string `json:"secret_key"`
		SecurityToken   string `json:"security_token"`
	} `json:"data"`
}

func (p *VaultProvider) Retrieve() (credentials.Value, error) {
	var value credentials.Value
	value.ProviderName = "vault"

	stsPath := []string{"v1", p.Engine, "creds", p.Role}
	response := awsResponse{}
	err := p.VaultClient.Read(stsPath, &response, nil)
	if err != nil {
		return value, errors.WithStack(err)
	}

	var (
		duration   = time.Duration(response.LeaseDuration) * time.Second
		expiration = time.Now().Add(duration)
		window     = duration / time.Duration(10) // Rotate after 90% of the lease time.
	)
	p.SetExpiration(expiration, window)

	value.AccessKeyID = response.Data.AccessKeyID
	value.SecretAccessKey = response.Data.SecretAccessKey
	value.SessionToken = response.Data.SecurityToken

	return value, nil
}
