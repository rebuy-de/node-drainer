package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
)

type Runner struct {
	awsProfile string
	sqsQueue   string
}

func (r *Runner) Bind(cmd *cobra.Command) error {
	cmd.PersistentFlags().StringVar(
		&r.awsProfile, "profile", "",
		`use a specific AWS profile from your credential file`)
	cmd.PersistentFlags().StringVar(
		&r.sqsQueue, "queue", "",
		`name of the SQS queue that contains the ASG lifecycle hook messages`)
	return nil
}

func (r *Runner) Run(ctx context.Context, cmd *cobra.Command, args []string) {
	ctx = InitIntrumentation(ctx)

	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           r.awsProfile,
	})
	cmdutil.Must(err)

	asgClient, err := asg.New(sess, r.sqsQueue)
	cmdutil.Must(err)

	ec2Client := ec2.New(sess, 1*time.Second)

	mainLoop := NewMainLoop(asgClient, ec2Client)

	server := &Server{
		ec2:      ec2Client,
		asg:      asgClient,
		mainloop: mainLoop,
	}

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(ec2Client.Run(ctx), "failed to run ec2 watcher")
	})
	egrp.Go(func() error {
		return errors.Wrap(asgClient.Run(ctx), "failed to run handler")
	})
	egrp.Go(func() error {
		return errors.Wrap(server.Run(ctx), "failed to run server")
	})
	egrp.Go(func() error {
		return errors.Wrap(mainLoop.Run(ctx), "failed to run main loop")
	})
	cmdutil.Must(egrp.Wait())
}
