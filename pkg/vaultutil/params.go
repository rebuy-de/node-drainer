package vaultutil

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	vault "github.com/mittwald/vaultgo"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type Params struct {
	Address string
	Role    string
	Token   string

	AWSRole       string
	AWSEnginePath string
}

func (p *Params) Bind(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(
		&p.Address, "vault-address", "",
		`Address of the Vault server.`)
	cmd.PersistentFlags().StringVar(
		&p.Role, "vault-role", cmdutil.Name,
		`Name of the Vault role.`)
	cmd.PersistentFlags().StringVar(
		&p.Token, "vault-token", "",
		`Token used for logging into Vault. If not preset, Kubernetes service account will be used.`)
}

func (p *Params) BindAWS(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(
		&p.AWSEnginePath, "vault-aws-engine-path", "aws",
		`Path to the AWS Secrets Engine.`)
	cmd.PersistentFlags().StringVar(
		&p.AWSRole, "vault-aws-role", cmdutil.Name,
		`Name of the role within the AWS Secrets Engine.`)
}

func (p *Params) Client() (*vault.Client, error) {
	opts := []vault.ClientOpts{}
	if p.Token != "" {
		opts = append(opts, vault.WithAuthToken(p.Token))
	} else {
		opts = append(opts, vault.WithKubernetesAuth(p.Role))
	}

	client, err := vault.NewClient(p.Address, vault.WithCaPath(""), opts...)
	return client, errors.WithStack(err)
}

func (p *Params) AWSCredentialProvider() (credentials.Provider, error) {
	client, err := p.Client()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &VaultProvider{
		Engine:      p.AWSEnginePath,
		Role:        p.AWSRole,
		VaultClient: client,
	}, nil
}

func (p *Params) AWSSession() (*session.Session, error) {
	cp, err := p.AWSCredentialProvider()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var (
		creds = credentials.NewCredentials(cp)
		conf  = &aws.Config{Credentials: creds}
	)

	return session.NewSession(conf)
}
