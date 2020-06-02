package cmd

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/cmdutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws"
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

	server := &Server{
		ec2Store:   ec2Store,
		asgHandler: handler,
	}

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
			ctx := logutil.Start(ctx, "loop")

			combined := aws.CombineInstances(
				handler.List(),
				ec2Store.List(),
			).Sort(aws.ByLaunchTime).SortReverse(aws.ByTriggeredAt)

			// Mark all instances as complete immediately.
			for _, instance := range combined.Select(aws.IsWaiting) {
				logutil.Get(ctx).
					WithFields(logFieldsFromStruct(instance)).
					Info("marking node as complete")

				err := handler.Complete(ctx, instance.InstanceID)
				if err != nil {
					return errors.Wrap(err, "failed to mark node as complete")
				}

				// Safe action that does not need a loop-restart.
				actionTaken.Emit()
			}

			// Tell instrumentation that an instance state changed.
			for _, instance := range combined {
				currState := instance.EC2.State
				prevState := stateCache[instance.InstanceID]

				if currState == prevState {
					continue
				}

				if prevState == "" {
					// It means there is no previous state. This might happen
					// after a restart.
					continue
				}

				l := logutil.Get(ctx).
					WithFields(logFieldsFromStruct(instance))

				//if currState == ec2.InstanceStateTerminated {
				//	err := handler.Delete(ctx, instance.InstanceID)
				//	if err != nil {
				//		return errors.Wrap(err, "failed to mark node as complete")
				//	}
				//}

				l.Debugf("instance state changed from '%s' to '%s'", prevState, currState)
				stateCache[instance.InstanceID] = currState

				// Safe action that does not need a loop-restart.
				actionTaken.Emit()
			}

			// Clean up old messages
			for _, instance := range combined.Filter(aws.HasEC2Data).Filter(aws.HasLifecycleMessage) {
				l := logutil.Get(ctx).
					WithFields(logFieldsFromStruct(instance))
				age := time.Since(instance.ASG.TriggeredAt)
				if age < 30*time.Minute {
					l.Warnf("termination time of %s was triggered just %v ago, assuming that the cache was empty",
						instance.InstanceID, age)
					continue
				}

				err := handler.Delete(ctx, instance.InstanceID)
				if err != nil {
					return errors.Wrap(err, "failed to delete message")
				}

				// Safe action that does not need a loop-restart.
				actionTaken.Emit()
			}

			return nil
		}()

	}

	return nil
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
