package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
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

	ec2Store := ec2.New(sess, 1*time.Second)

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

func (r *Runner) runMainLoop(ctx context.Context, handler asg.Handler, ec2Store *ec2.Store) error {
	ctx = logutil.Start(ctx, "mainloop")

	// When do an action inside the loop we exit the loop and restart it. Since
	// we need to ensure that the loop get run again, we need to signal it.
	actionTaken := new(syncutil.SignalEmitter)

	signaler := syncutil.SignalerFromEmitters(
		actionTaken,
		handler.SignalEmitter(),
		ec2Store.SignalEmitter(),
	)

	stateCache := map[string]string{}

	for ctx.Err() == nil {
		<-signaler.C(ctx, time.Minute)

		func() error {
			ctx := logutil.Start(ctx, "step")

			waitingInstances := handler.List(asg.IsWaiting)
			if len(waitingInstances) > 0 {
				logutil.Get(ctx).
					Infof("%d instances are waiting in the ASG Lifecycle queue", len(waitingInstances))

				instance := waitingInstances[0]
				logutil.Get(ctx).
					WithFields(logrus.Fields{
						"instance-id":  instance.ID,
						"triggered-at": instance.TriggeredAt,
						"completed-at": instance.CompletedAt,
						"deleted-at":   instance.DeletedAt,
					}).Info("marking node as complete")

				err := handler.Complete(ctx, instance.ID)
				if err != nil {
					return errors.Wrap(err, "failed to mark node as complete")
				}

				actionTaken.Emit()
				return nil
			}

			logutil.Get(ctx).Debug("no instances waiting in the ASG Lifecycle queue")

			for _, instance := range ec2Store.List() {
				currState := instance.State
				prevState := stateCache[instance.InstanceID]

				if currState == prevState {
					continue
				}

				l := logutil.Get(ctx).WithFields(logrus.Fields{
					"instance-id": instance.InstanceID,
				})

				if currState == ec2.InstanceStateTerminated {
					err := handler.Delete(ctx, instance.InstanceID)
					if err != nil {
						return errors.Wrap(err, "failed to mark node as complete")
					}
				}

				l.Debugf("instance state changed from '%s' to '%s'", prevState, currState)
				stateCache[instance.InstanceID] = currState
				actionTaken.Emit()
				return nil
			}

			logutil.Get(ctx).Debug("no instances changed their state")

			return nil
		}()
	}

	return nil
}
