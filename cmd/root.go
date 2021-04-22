package cmd

import (
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/cmdutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	r := new(Runner)

	return cmdutil.New(
		"node-drainer", "Reads AWS ASG Lifecycle Events and drains Kubernetes nodes",
		r.Bind,
		cmdutil.WithLogVerboseFlag(),
		cmdutil.WithLogToGraylog(),
		cmdutil.WithVersionCommand(),
		cmdutil.WithVersionLog(logrus.InfoLevel),
		cmdutil.WithRun(r.Run),
	)
}
