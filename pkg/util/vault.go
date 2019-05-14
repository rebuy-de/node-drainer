package util

import (
	"io/ioutil"

	vault "github.com/hashicorp/vault/api"
	log "github.com/sirupsen/logrus"
)

func FetchVaultClient(vaultAddress string) (*vault.Client, *vault.Secret, error) {
	client, err := vault.NewClient(&vault.Config{
		Address: vaultAddress,
	})
	if err != nil {
		return &vault.Client{}, &vault.Secret{}, err
	}

	serviceAccountTokenArray, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return &vault.Client{}, &vault.Secret{}, err
	}
	serviceAccountToken := string(serviceAccountTokenArray[:])
	log.Debugf("SAT: %s\n", serviceAccountToken)

	data := make(map[string]interface{})
	data["role"] = "node_drainer"
	data["jwt"] = serviceAccountToken

	logical := client.Logical()
	secret, err := logical.Write("auth/kubernetes/login", data)
	if err != nil {
		return &vault.Client{}, &vault.Secret{}, err
	}
	client.SetToken(secret.Auth.ClientToken)
	log.Debugf("Client token %s acquired, valid for %d seconds", secret.Auth.ClientToken, secret.Auth.LeaseDuration)
	return client, secret, nil
}

// Used for dynamic credentials
func CreateVaultRenewer(client *vault.Client, secret *vault.Secret) (*vault.Renewer, error) {
	renewer, err := client.NewRenewer(&vault.RenewerInput{
		Secret: secret,
	})
	if err != nil {
		return &vault.Renewer{}, err
	}
	go renewer.Renew()
	go func(renewer *vault.Renewer) {
		for {
			select {
			case err := <-renewer.DoneCh():
				if err != nil {
					log.Fatal(err)
				}
			case renewal := <-renewer.RenewCh():
				log.Printf("Successfully renewed vault token: %s", renewal.RenewedAt.Format("15:04:05"))
			}
		}
	}(renewer)
	return renewer, nil
}

// Used for dynamic credentials
func GenerateAWSCredentials(client *vault.Client) (AWSProfile, int, error) {
	logical := client.Logical()
	creds, err := logical.Read("/aws/creds/node_drainer")
	if err != nil {
		return AWSProfile{}, 0, err
	}
	log.Debugf("AWS credentials acquired - ID: %s Secret: %s - valid for %d seconds - LeaseID: %s", creds.Data["access_key"], creds.Data["secret_key"], creds.LeaseDuration, creds.LeaseID)

	return AWSProfile{
		AccessKeyID:     creds.Data["access_key"].(string),
		SecretAccessKey: creds.Data["secret_key"].(string),
	}, creds.LeaseDuration, nil
}

// Used for static credentials
func FetchAWSCredentials(client *vault.Client) (AWSProfile, error) {
	logical := client.Logical()
	creds, err := logical.Read("/secret/data/node_drainer")
	if err != nil {
		return AWSProfile{}, err
	}
	data := creds.Data["data"].(map[string]interface{})
	log.Debugf("AWS credentials acquired - ID: %s Secret: %s", data["access_key"], data["secret_key"])

	return AWSProfile{
		AccessKeyID:     data["access_key"].(string),
		SecretAccessKey: data["secret_key"].(string),
	}, nil
}
