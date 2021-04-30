package vaultutil

import (
	"github.com/hashicorp/vault/api"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
)

func prettyPrintSecret(original *api.Secret) string {
	// The struct gets cloned by hand, because it contains some pointers and we
	// want to avoid redacting data in the original secret. Additionally
	// actually cloning it and redacting sensitive information after, would
	// mean that new fields are visible by default.
	var (
		secret api.Secret
		auth   api.SecretAuth
		data   = map[string]interface{}{}
	)

	for k, v := range original.Data {
		switch k {
		case "access_key": // Keep AWS Access Key ID.
		default:
			v = "[REDACTED]"
		}
		data[k] = v
	}

	if original.Auth != nil {
		auth.Policies = original.Auth.Policies
		auth.TokenPolicies = original.Auth.TokenPolicies
		auth.IdentityPolicies = original.Auth.IdentityPolicies
		auth.Metadata = original.Auth.Metadata
		auth.Orphan = original.Auth.Orphan
		auth.EntityID = original.Auth.EntityID
		auth.LeaseDuration = original.Auth.LeaseDuration
		auth.Renewable = original.Auth.Renewable

		if original.Auth.ClientToken != "" {
			auth.ClientToken = "[REDACTED]"
		}

		if original.Auth.Accessor != "" {
			auth.Accessor = "[REDACTED]"
		}
	}

	secret.RequestID = original.RequestID
	secret.LeaseID = original.LeaseID
	secret.LeaseDuration = original.LeaseDuration
	secret.Renewable = original.Renewable
	secret.Data = data
	secret.Warnings = original.Warnings
	secret.Auth = &auth

	return logutil.PrettyPrint(secret)
}
