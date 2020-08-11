package cmd

import (
	"context"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/instutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

const (
	metricMainLoopActions          = "mainloop_actions_total"
	metricMainLoopDrainDuration    = "mainloop_drain_duration"
	metricMainLoopIterations       = "mainloop_iterations_total"
	metricMainLoopPendingInstances = "mainloop_pending_instances"
	metricMainLoopPodStats         = "mainloop_pod_stats"
)

type instCacheKey string

const instCacheKeyStates instCacheKey = "ec2-instance-state-cache"

func InitIntrumentation(ctx context.Context) context.Context {
	ctx = instutil.NewCounterVec(ctx, metricMainLoopActions, "action")
	ctx = instutil.NewHistogram(ctx, metricMainLoopDrainDuration,
		instutil.BucketScale(60, 1, 2, 3, 5, 8, 13, 21, 34)...)
	ctx = instutil.NewCounter(ctx, metricMainLoopIterations)
	ctx = instutil.NewGauge(ctx, metricMainLoopPendingInstances)
	ctx = instutil.NewGaugeVec(ctx, metricMainLoopPodStats, "name")

	// Register the already known label values, so Prometheus starts with 0 and
	// not 1 and properly calculates rates.
	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("noop").Add(0)
		c.WithLabelValues("lifecycle-complete").Add(0)
		c.WithLabelValues("lifecycle-delete").Add(0)
	}

	gv, ok := instutil.GaugeVec(ctx, metricMainLoopPodStats)
	if ok {
		gv.WithLabelValues("total").Add(0)
		gv.WithLabelValues("need-eviction").Add(0)
		gv.WithLabelValues("ready-eviction").Add(0)
	}

	cache := map[string]string{}
	ctx = context.WithValue(ctx, instCacheKeyStates, &cache)

	return ctx
}

func InstMainLoopStarted(ctx context.Context, instances collectors.Instances, pods collectors.Pods) {
	c, ok := instutil.Counter(ctx, metricMainLoopIterations)
	if ok {
		c.Inc()
	}

	// Log instance stats
	g, ok := instutil.Gauge(ctx, metricMainLoopPendingInstances)
	if ok {
		// Note: In the future this should track all instances that have a
		// lifecycle message and are not completed yet. But since we are now
		// still watching the old node-drainer, this schould be fine.
		g.Set(float64(len(instances.
			Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
			Select(collectors.HasLifecycleMessage),
		)))
	}

	// Log pod stats
	var (
		podsThatNeedEviction = SelectPodsThatNeedEviction(pods)
		podsReadyForEviction = SelectPodsReadyForEviction(pods)
	)

	gv, ok := instutil.GaugeVec(ctx, metricMainLoopPodStats)
	if ok {
		gv.WithLabelValues("total").Set(float64(len(pods)))
		gv.WithLabelValues("need-eviction").Set(float64(len(podsThatNeedEviction)))
		gv.WithLabelValues("ready-eviction").Set(float64(len(podsReadyForEviction)))
	}
	if len(podsThatNeedEviction) > 0 {
		logutil.Get(ctx).
			Debugf("there are %d pods that need eviction", len(podsThatNeedEviction))
	}

	if len(podsThatNeedEviction) > 0 {
		logutil.Get(ctx).
			Debugf("there are %d pods that are ready for eviction", len(podsReadyForEviction))
	}

	// Log changed instance states
	cache, ok := ctx.Value(instCacheKeyStates).(*map[string]string)
	if ok {
		for _, instance := range instances {
			logger := logutil.Get(ctx).
				WithFields(logutil.FromStruct(instance))

			currState := instance.EC2.State
			prevState := (*cache)[instance.InstanceID]

			(*cache)[instance.InstanceID] = currState

			if currState == prevState {
				continue
			}

			if prevState == "" {
				// It means there is no previous state. This might happen
				// after a restart.
				continue
			}

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
	}

}

func InstMainLoopNoop(ctx context.Context) {
	logutil.Get(ctx).Debug("mainloop finished without action")

	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("noop").Inc()
	}
}

func InstMainLoopCompletingInstance(ctx context.Context, instance collectors.Instance) {
	logutil.Get(ctx).
		WithFields(logutil.FromStruct(instance)).
		Info("marking node as complete")

	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("lifecycle-complete").Inc()
	}
}

func InstMainLoopDeletingLifecycleMessage(ctx context.Context, instance collectors.Instance) {
	logutil.Get(ctx).
		WithFields(logutil.FromStruct(instance)).
		Info("deleting lifecycle message from SQS")

	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("lifecycle-delete").Inc()
	}
}

func InstMainLoopDeletingLifecycleMessageAgeSanityCheckFailed(ctx context.Context, instance collectors.Instance, age time.Duration) {
	logutil.Get(ctx).
		WithFields(logutil.FromStruct(instance)).
		Warnf("termination time of %s was triggered just %v ago, assuming that the cache was empty",
			instance.InstanceID, age)
}

func InstMainLoopEvictPod(ctx context.Context, pod collectors.Pod) {
	logutil.Get(ctx).
		WithFields(logutil.FromStruct(pod)).
		Warnf("would evict pod %s", pod.Name)
}
