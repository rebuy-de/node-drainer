package cmd

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rebuy-de/node-drainer/v2/pkg/instutil"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

const (
	metricMainLoopIterations       = "mainloop_iterations_total"
	metricMainLoopDrainDuration    = "mainloop_drain_duration"
	metricMainLoopPendingInstances = "mainloop_pending_instances"
)

type metrics struct {
	mainloopIteration prometheus.Counter
}

func InitIntrumentation(ctx context.Context) context.Context {
	ctx = instutil.NewCounter(ctx, metricMainLoopIterations)
	ctx = instutil.NewGauge(ctx, metricMainLoopPendingInstances)
	ctx = instutil.NewHistogram(ctx, metricMainLoopDrainDuration,
		instutil.BucketScale(60, 1, 2, 3, 5, 8, 13, 21, 34)...)
	return ctx
}

func InstMainLoopStarted(ctx context.Context, instances aws.Instances) {
	c, ok := instutil.Counter(ctx, metricMainLoopIterations)
	if ok {
		c.Inc()
	}

	g, ok := instutil.Gauge(ctx, metricMainLoopPendingInstances)
	if ok {
		// Note: In the future this should track all instances that have a
		// lifecycle message and are not completed yet. But since we are now
		// still watching the old node-drainer, this schould be fine.
		g.Set(float64(len(instances.
			Select(aws.HasEC2State(ec2.InstanceStateRunning)).
			Select(aws.HasLifecycleMessage),
		)))
	}

}

func InstMainLoopCompletingInstance(ctx context.Context, instance aws.Instance) {
	logutil.Get(ctx).
		WithFields(logFieldsFromStruct(instance)).
		Info("marking node as complete")
}

func InstMainLoopInstanceStateChanged(ctx context.Context, instance aws.Instance, prevState, currState string) {
	logger := logutil.Get(ctx).
		WithFields(logFieldsFromStruct(instance))

	logger.Infof("instance state changed from '%s' to '%s'", prevState, currState)

	if currState == ec2.InstanceStateTerminated {
		duration := instance.EC2.TerminationTime.Sub(instance.ASG.TriggeredAt)
		logger.Infof("instance drainage took %v", duration)

		m, ok := instutil.Histogram(ctx, metricMainLoopDrainDuration)
		if ok {
			m.Observe(duration.Seconds())
		}
	}
}

func InstMainLoopDeletingLifecycleMessage(ctx context.Context, instance aws.Instance) {
	logutil.Get(ctx).
		WithFields(logFieldsFromStruct(instance)).
		Info("deleting lifecycle message from SQS")
}

func InstMainLoopDeletingLifecycleMessageAgeSanityCheckFailed(ctx context.Context, instance aws.Instance, age time.Duration) {
	logutil.Get(ctx).
		WithFields(logFieldsFromStruct(instance)).
		Warnf("termination time of %s was triggered just %v ago, assuming that the cache was empty",
			instance.InstanceID, age)
}
