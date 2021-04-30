package vaultutil

import (
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
