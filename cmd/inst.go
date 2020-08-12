package cmd

import (
	"context"
	"path"
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
	ctx = instutil.NewTransitionCollector(ctx, "pod-eviction")

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
		gv.WithLabelValues("eviction-want").Add(0)
		gv.WithLabelValues("eviction-ready").Add(0)
		gv.WithLabelValues("eviction-unready").Add(0)
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

	var (
		podsThatWantEviction   = pods.Select(PodsThatWantEviction())
		podsReadyForEviction   = pods.Select(PodsReadyForEviction())
		podsUnreadyForEviction = pods.Select(PodsUnreadyForEviction())
	)

	// Log pod stats
	gv, ok := instutil.GaugeVec(ctx, metricMainLoopPodStats)
	if ok {
		gv.WithLabelValues("total").Set(float64(len(pods)))
		gv.WithLabelValues("eviction-want").Set(float64(len(podsThatWantEviction)))
		gv.WithLabelValues("eviction-ready").Set(float64(len(podsReadyForEviction)))
		gv.WithLabelValues("eviction-unready").Set(float64(len(podsUnreadyForEviction)))
	}
	if len(podsThatWantEviction) > 0 {
		logutil.Get(ctx).
			WithField("eviction-want", len(podsThatWantEviction)).
			WithField("eviction-ready", len(podsReadyForEviction)).
			WithField("eviction-unready", len(podsUnreadyForEviction)).
			Debugf("there are %d pods that want eviction (%d ready, %d unready)",
				len(podsThatWantEviction), len(podsReadyForEviction), len(podsUnreadyForEviction),
			)
	}

	tc := instutil.GetTransitionCollector(ctx, "pod-eviction")
	for _, pod := range podsThatWantEviction {
		name := path.Join(pod.Namespace, pod.Name)

		switch {
		case PodsReadyForEviction()(&pod):
			tc.Observe(name, "eviction-ready", logutil.FromStruct(pod))
		case PodsUnreadyForEviction()(&pod):
			tc.Observe(name, "eviction-unready", logutil.FromStruct(pod))
		}
	}
	for _, transition := range tc.Finish() {
		logutil.Get(ctx).
			WithFields(transition.Fields).
			Infof("pod %s changed state from '%s' to '%s'",
				transition.Name, transition.From, transition.To)
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
