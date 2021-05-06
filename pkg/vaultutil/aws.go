package vaultutil

import (
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
)

type awsCredentialsProvider struct {
	manager *Manager
	credentials.Expiry
}

func (p *awsCredentialsProvider) Retrieve() (credentials.Value, error) {
	var value credentials.Value
	value.ProviderName = "vault"

	secret, err := p.manager.client.Logical().Read(path.Join(p.manager.params.AWSEnginePath, "creds", p.manager.params.AWSRole))
	if err != nil {
		return value, errors.WithStack(err)
	}

	logutil.Get(p.manager.ctx).
		WithField("secret-data", prettyPrintSecret(secret)).
		Debugf("created new AWS lease")

	var (
		duration   = time.Duration(secret.LeaseDuration) * time.Second
		expiration = time.Now().Add(duration)
		window     = duration / time.Duration(10) // Rotate after 90% of the lease time.
	)
	p.SetExpiration(expiration, window)

	// The blank identifier avoids throwing a panic, if the data is not a
	// string for some reason.
	value.AccessKeyID, _ = secret.Data["access_key"].(string)
	value.SecretAccessKey, _ = secret.Data["secret_key"].(string)
	value.SessionToken, _ = secret.Data["security_token"].(string)

	return value, nil
}
