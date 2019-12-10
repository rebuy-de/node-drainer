package cmd

import (
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	r := new(Runner)

	return cmdutil.New(
		"node-drainer", "A Slack integration to manage k8s deployments.",
		r.Bind,
		cmdutil.WithLogVerboseFlag(),
		cmdutil.WithVersionCommand(),
		cmdutil.WithVersionLog(logrus.InfoLevel),
		cmdutil.WithRun(r.Run),
	)
}
