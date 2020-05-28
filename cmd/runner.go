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

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/syncutil"
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

	ec2Store := ec2.New(sess, 10*time.Second)

	egrp, ctx := errgroup.WithContext(ctx)
	egrp.Go(func() error {
		return errors.Wrap(ec2Store.Run(ctx), "failed to run ec2 watcher")
	})
	egrp.Go(func() error {
		return errors.Wrap(handler.Run(ctx), "failed to run handler")
	})
	egrp.Go(func() error {
		return errors.Wrap(r.runMainLoop(ctx, handler, ec2Store), "failed to run main loop")
	})
	cmdutil.Must(egrp.Wait())
}

func (r *Runner) runMainLoop(ctx context.Context, handler asg.Handler, ec2 *ec2.Store) error {
	l := logrus.WithField("subsystem", "mainloop")

	signaler := syncutil.SignalerFromEmitters(
		handler.SignalEmitter(),
		ec2.SignalEmitter(),
	)

	for ctx.Err() == nil {
		<-signaler.C(ctx, time.Minute)

		instances := handler.List()
		if len(instances) == 0 {
			l.Debug("no instances waiting for shutdown")
			continue
		}

		l.Infof("%d instances are waiting for shutdown", len(instances))
		for _, instance := range instances {
			l := l.WithFields(logrus.Fields{
				"instance-id":  instance.ID,
				"triggered-at": instance.TriggeredAt,
				"completed-at": instance.CompletedAt,
				"deleted-at":   instance.DeletedAt,
			})
			l.Debugf("%s is waiting for shutdown", instance.ID)
		}

		instance := instances[0]
		l.WithFields(logrus.Fields{
			"instance-id":  instance.ID,
			"triggered-at": instance.TriggeredAt,
			"completed-at": instance.CompletedAt,
			"deleted-at":   instance.DeletedAt,
		}).Info("marking node as complete")
		err := handler.Complete(ctx, instance.ID)
		if err != nil {
			return errors.Wrap(err, "failed to mark node as complete")
		}
	}

	return nil
}
