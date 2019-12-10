package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

type Runner struct{}

func (r *Runner) Bind(cmd *cobra.Command) error {
	return nil
}

func (r *Runner) Run(ctx context.Context, cmd *cobra.Command, args []string) {
}
