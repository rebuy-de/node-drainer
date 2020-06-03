package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/sirupsen/logrus"
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
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Profile:           r.awsProfile,
	})
	cmdutil.Must(err)

	handler, err := asg.NewHandler(sess, r.sqsQueue)
	cmdutil.Must(err)

	ec2Store := ec2.New(sess, 1*time.Second)

	server := &Server{
		ec2Store:   ec2Store,
		asgHandler: handler,
	}

	mainLoop := NewMainLoop(handler, ec2Store)

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(ec2Store.Run(ctx), "failed to run ec2 watcher")
	})
	egrp.Go(func() error {
		return errors.Wrap(handler.Run(ctx), "failed to run handler")
	})
	egrp.Go(func() error {
		return errors.Wrap(server.Run(ctx), "failed to run server")
	})
	egrp.Go(func() error {
		return errors.Wrap(mainLoop.Run(ctx), "failed to run main loop")
	})
	cmdutil.Must(egrp.Wait())
}

func logFieldsFromStruct(s interface{}) logrus.Fields {
	fields := logrus.Fields{}
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "logfield",
		Result:  &fields,
	})
	if err != nil {
		return logrus.Fields{"logfield-error": err}
	}

	err = dec.Decode(s)
	if err != nil {
		return logrus.Fields{"logfield-error": err}
	}

	return fields
}
