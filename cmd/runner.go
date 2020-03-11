package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/asg"
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

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(handler.Run(ctx), "failed to run handler")
	})
	egrp.Go(func() error {
		return errors.Wrap(r.runMainLoop(ctx, handler), "failed to run main loop")
	})
	cmdutil.Must(egrp.Wait())
}

func (r *Runner) runMainLoop(ctx context.Context, handler asg.Handler) error {
	l := logrus.WithField("subsystem", "mainloop")

	for ctx.Err() == nil {
		instances := handler.List()
		if len(instances) == 0 {
			l.Info("no instances waiting for shutdown")
			time.Sleep(5 * time.Second)
			continue
		}

		l.Infof("%d instances are waiting for shutdown", len(instances))
		for _, instance := range instances {
			l := l.WithFields(logrus.Fields{
				"node_name":   instance.Name,
				"instance_id": instance.ID,
				"time":        instance.Time,
			})
			l.Debugf("%s is waiting for shutdown", instance.Name)
		}

		instance := instances[0]
		l.WithFields(logrus.Fields{
			"node_name":   instance.Name,
			"instance_id": instance.ID,
			"time":        instance.Time,
		}).Info("marking node as complete")
		err := handler.Complete(instance.ID)
		if err != nil {
			return errors.Wrap(err, "failed to mark node as complete")
		}

		time.Sleep(time.Second)
	}

	return nil
}
