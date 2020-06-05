package cmd

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/syncutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

// MainLoop does the actual node-drainer actions. When any client cache
// changes, it starts a new update loop and checks whether an action is
// required.
type MainLoop struct {
	stateCache  map[string]string
	triggerLoop *syncutil.SignalEmitter
	signaler    syncutil.Signaler

	failureCount int

	asg asg.Client
	ec2 ec2.Client
}

// NewMainLoop initializes a MainLoop.
func NewMainLoop(asgClient asg.Client, ec2Client ec2.Client) *MainLoop {
	ml := new(MainLoop)

	ml.stateCache = map[string]string{}
	ml.asg = asgClient
	ml.ec2 = ec2Client
	ml.triggerLoop = new(syncutil.SignalEmitter)

	ml.signaler = syncutil.SignalerFromEmitters(
		ml.triggerLoop,
		asgClient.SignalEmitter(),
		ec2Client.SignalEmitter(),
	)

	return ml
}

// Healthy indicates whether the background job is running correctly.
func (l *MainLoop) Healthy() bool {
	return l.failureCount == 0
}

// Run starts the mainloop.
func (l *MainLoop) Run(ctx context.Context) error {
	ctx = logutil.Start(ctx, "mainloop")

	logutil.Get(ctx).Debug("waiting for EC2 cache to warm up")
	for ctx.Err() == nil {
		if len(l.ec2.List()) > 0 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}
	logutil.Get(ctx).Debug("waiting for EC2 cache done")

	for ctx.Err() == nil {
		err := l.runOnce(ctx)
		if err != nil {
			logutil.Get(ctx).
				WithError(errors.WithStack(err)).
				Errorf("main loop run failed %d times in a row", l.failureCount)
			l.failureCount++

			// Sleep shortly because we do not want to DoS our logging system.
			time.Sleep(100 * time.Millisecond)
		} else {
			l.failureCount = 0
		}

		<-l.signaler.C(ctx, time.Minute)
	}

	return nil
}

func (l *MainLoop) runOnce(ctx context.Context) error {
	ctx = logutil.Start(ctx, "loop")

	combined := aws.CombineInstances(
		l.asg.List(),
		l.ec2.List(),
	).Sort(aws.ByLaunchTime).SortReverse(aws.ByTriggeredAt)

	// Mark all instances as complete immediately.
	for _, instance := range combined.Select(aws.IsWaiting) {
		logutil.Get(ctx).
			WithFields(logFieldsFromStruct(instance)).
			Info("marking node as complete")

		err := l.asg.Complete(ctx, instance.InstanceID)
		if err != nil {
			return errors.Wrap(err, "failed to mark node as complete")
		}

		// Safe action that does not need a loop-restart.
		l.triggerLoop.Emit()
	}

	// Tell instrumentation that an instance state changed.
	for _, instance := range combined {
		currState := instance.EC2.State
		prevState := l.stateCache[instance.InstanceID]

		l.stateCache[instance.InstanceID] = currState

		if currState == prevState {
			continue
		}

		if prevState == "" {
			// It means there is no previous state. This might happen
			// after a restart.
			continue
		}

		logger := logutil.Get(ctx).
			WithFields(logFieldsFromStruct(instance))

		logger.Infof("instance state changed from '%s' to '%s'", prevState, currState)

		if currState == ec2.InstanceStateTerminated {
			logger.Infof(
				"instance drainage took %v",
				instance.EC2.TerminationTime.Sub(instance.ASG.TriggeredAt),
			)
		}

		// Safe action that does not need a loop-restart.
		l.triggerLoop.Emit()
	}

	// Clean up old messages
	for _, instance := range combined.Filter(aws.HasEC2Data).Select(aws.HasLifecycleMessage) {
		logger := logutil.Get(ctx).
			WithFields(logFieldsFromStruct(instance))
		age := time.Since(instance.ASG.TriggeredAt)
		if age < 30*time.Minute {
			logger.Warnf("termination time of %s was triggered just %v ago, assuming that the cache was empty",
				instance.InstanceID, age)
			l.triggerLoop.Emit() // we need to retry
			continue
		}

		err := l.asg.Delete(ctx, instance.InstanceID)
		if err != nil {
			return errors.Wrap(err, "failed to delete message")
		}

		logger.Info("deleted lifecycle message from SQS")

		// Safe action that does not need a loop-restart.
		l.triggerLoop.Emit()
	}

	return nil
}
