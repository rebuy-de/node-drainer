package cmd

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
)

// MainLoop does the actual node-drainer actions. When any client cache
// changes, it starts a new update loop and checks whether an action is
// required.
type MainLoop struct {
	stateCache  map[string]string
	triggerLoop *syncutil.SignalEmitter
	signaler    syncutil.Signaler

	failureCount int

	collectors collectors.Collectors
}

// NewMainLoop initializes a MainLoop.
func NewMainLoop(collectors collectors.Collectors) *MainLoop {
	ml := new(MainLoop)

	ml.stateCache = map[string]string{}
	ml.collectors = collectors
	ml.triggerLoop = new(syncutil.SignalEmitter)

	ml.signaler = syncutil.SignalerFromEmitters(
		ml.triggerLoop,
		ml.collectors.EC2.SignalEmitter(),
		ml.collectors.ASG.SignalEmitter(),
		ml.collectors.Spot.SignalEmitter(),
		//ml.collectors.Node.SignalEmitter(),
		//ml.collectors.Pod.SignalEmitter(),
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
		if len(l.collectors.EC2.List()) > 0 {
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
		} else {
			l.failureCount = 0
		}

		time.Sleep(1 * time.Second)

		<-l.signaler.C(ctx, time.Minute)
	}

	return nil
}

func (l *MainLoop) runOnce(ctx context.Context) error {
	// Implementation Detail: This function uses a lot of for loops that
	// iterate of the filtered instances. While we mostly return on the first
	// iteration, it still makes sense to use the loops, because:
	//   * No need to check explicitly whether the filtered list is empty.
	//   * Possibility to skip an item when some additional condition is not met.
	// Also it makes sense to be consistent with the usage of loops, because it
	// makes the code more readable (ie visual separation of steps by loop
	// blocks).

	// Another One: A call of this function should only do a single action (eg
	// evict a pod or complete an instance lifecycle). This is, because the
	// action can change the underlying data, but this is not reflected in the
	// local variables. Additionally it is easier to restart the whole thing
	// after any condition is met than having to keep track about possible side
	// effects of any action. One example:
	// * Evicting a pod makes all other pods with the same owner (eg
	//   Deployment) unevictable, but that data is not updated in the local
	//   variables. We could implement things to make it update, but is easier
	//   to just restart the loop every time.

	ctx = logutil.Start(ctx, "loop")

	instances, pods := collectors.Combine(l.collectors.List(ctx))
	instances = instances.
		Sort(collectors.InstancesByLaunchTime).SortReverse(collectors.InstancesByTriggeredAt)

	InstMainLoopStarted(ctx, instances, pods)

	for _, instance := range instances.Select(InstancesThatNeedLifecycleCompletion()) {
		InstMainLoopCompletingInstance(ctx, instance)

		err := l.collectors.ASG.Complete(ctx, instance.InstanceID)
		if err != nil {
			return errors.Wrap(err, "failed to mark node as complete")
		}

		l.triggerLoop.Emit()
		return nil
	}

	for _, instance := range instances.Select(InstancesThanNeedLifecycleDeletion()) {
		InstMainLoopDeletingLifecycleMessage(ctx, instance)

		age := time.Since(instance.ASG.TriggeredAt)
		if age < 30*time.Minute {
			InstMainLoopDeletingLifecycleMessageAgeSanityCheckFailed(ctx, instance, age)
			l.triggerLoop.Emit() // we need to retry
			continue
		}

		err := l.collectors.ASG.Delete(ctx, instance.InstanceID)
		if err != nil {
			return errors.Wrap(err, "failed to delete message")
		}

		l.triggerLoop.Emit()
		return nil
	}

	InstMainLoopNoop(ctx)
	return nil
}
