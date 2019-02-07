package util

import (
	vault "github.com/hashicorp/vault/api"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
)

func GenerateAWSCredentials(vaultAddress string) (AWSProfile, error) {
	client, err := vault.NewClient(&vault.Config{
		Address: vaultAddress,
	})
	if err != nil {
		return AWSProfile{}, err
	}

	serviceAccountTokenArray, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return AWSProfile{}, err
	}
	serviceAccountToken := string(serviceAccountTokenArray[:])
	log.Debugf("SAT: %s\n", serviceAccountToken)

	data := make(map[string]interface{})
	data["role"] = "node_drainer"
	data["jwt"] = serviceAccountToken

	logical := client.Logical()
	secret, err := logical.Write("auth/kubernetes/login", data)
	if err != nil {
		return AWSProfile{}, err
	}
	log.Debugf("Client token %s acquired, valid for %d seconds\n", secret.Auth.ClientToken, secret.Auth.LeaseDuration)
	client.SetToken(secret.Auth.ClientToken)

	logical = client.Logical()
	creds, _ := logical.Read("/aws/creds/node_drainer")
	log.Debugf("AWS credentials acquired:\n  - %s\n  - %s\nvalid for %d seconds\n", creds.Data["access_key"], creds.Data["secret_key"], creds.LeaseDuration)
	return AWSProfile{
		AccessKeyID:     creds.Data["access_key"].(string),
		SecretAccessKey: creds.Data["secret_key"].(string),
	}, nil
}
