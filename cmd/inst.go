package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/instutil"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
)

const (
	metricMainLoopActions                  = "mainloop_actions_total"
	metricMainLoopDrainDuration            = "mainloop_drain_duration"
	metricMainLoopInstanceStateTransitions = "mainloop_instance_state_transitions_total"
	metricMainLoopIterations               = "mainloop_iterations_total"
	metricMainLoopPendingInstances         = "mainloop_pending_instances"
	metricMainLoopPodStats                 = "mainloop_pod_stats"
	metricMainLoopPodTransitions           = "mainloop_pod_transitions_total"
	metricMainLoopSpotStateTransitions     = "mainloop_spot_transitions_total"
)

func InitIntrumentation(ctx context.Context) context.Context {
	ctx = instutil.NewCounterVec(ctx, metricMainLoopActions, "action")
	ctx = instutil.NewHistogram(ctx, metricMainLoopDrainDuration,
		instutil.BucketScale(60, 1, 2, 3, 5, 8, 13, 21, 34)...)
	ctx = instutil.NewCounter(ctx, metricMainLoopIterations)
	ctx = instutil.NewGauge(ctx, metricMainLoopPendingInstances)
	ctx = instutil.NewGaugeVec(ctx, metricMainLoopPodStats, "name")

	ctx = instutil.NewCounterVec(ctx, metricMainLoopPodTransitions, "from", "to")
	ctx = instutil.NewCounterVec(ctx, metricMainLoopInstanceStateTransitions, "from", "to")
	ctx = instutil.NewCounterVec(ctx, metricMainLoopSpotStateTransitions, "from", "to")

	ctx = instutil.NewTransitionCollector(ctx, metricMainLoopPodTransitions)
	ctx = instutil.NewTransitionCollector(ctx, metricMainLoopInstanceStateTransitions)
	ctx = instutil.NewTransitionCollector(ctx, metricMainLoopSpotStateTransitions)

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

	cv, ok := instutil.GaugeVec(ctx, metricMainLoopPodTransitions)
	if ok {
		values := []string{"", "eviction-ready", "eviction-unready"}
		for _, from := range values {
			for _, to := range values {
				cv.WithLabelValues(from, to).Add(0)
			}
		}
	}

	cv, ok = instutil.GaugeVec(ctx, metricMainLoopInstanceStateTransitions)
	if ok {
		values := []string{
			ec2.InstanceStatePending,
			ec2.InstanceStateRunning,
			ec2.InstanceStateTerminated,
			ec2.InstanceStateShuttingDown,
		}
		for _, from := range values {
			for _, to := range values {
				cv.WithLabelValues(from, to).Add(0)
			}
		}
	}

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

	// Log pod changes
	tcp := instutil.GetTransitionCollector(ctx, metricMainLoopPodTransitions)
	for _, pod := range podsThatWantEviction {
		name := path.Join(pod.Namespace, pod.Name)

		switch {
		case PodsReadyForEviction()(&pod):
			tcp.Observe(name, "eviction-ready", logutil.FromStruct(pod))
		case PodsUnreadyForEviction()(&pod):
			tcp.Observe(name, "eviction-unready", logutil.FromStruct(pod))
		}
	}
	for _, transition := range tcp.Finish() {
		var (
			from = transition.From
			to   = transition.To
		)

		if from == "" {
			from = "N/A"
		}
		if to == "" {
			to = "N/A"
		}

		logutil.Get(ctx).
			WithField(
				"pod-status-transition",
				fmt.Sprintf("%s -> %s", transition.From, transition.To),
			).
			WithFields(transition.Fields).
			Infof("pod %s changed state: [ %s -> %s ]",
				transition.Name, from, to)

		cv, ok := instutil.CounterVec(ctx, metricMainLoopPodTransitions)
		if ok {
			cv.WithLabelValues(from, to).Inc()
		}
	}

	// Log ec2 state changes
	tci := instutil.GetTransitionCollector(ctx, metricMainLoopInstanceStateTransitions)
	for _, instance := range instances {
		tci.Observe(instance.InstanceID, instance.EC2.State, logutil.FromStruct(instance))
	}
	for _, transition := range tci.Finish() {
		logger := logutil.Get(ctx).
			WithField(
				"ec2-state-transition",
				fmt.Sprintf("%s -> %s", transition.From, transition.To),
			).
			WithFields(transition.Fields)

		if transition.From == "" || transition.To == "" {
			// These transitions are not interesting. We log them in debug
			// level for completeness anyways.
			logger.Debugf("instance %s changed state: [ %s -> %s ]",
				transition.Name, transition.From, transition.To)
			continue
		}

		logger.Infof("instance %s changed state: [ %s -> %s ]",
			transition.Name, transition.From, transition.To)

		cv, ok := instutil.CounterVec(ctx, metricMainLoopInstanceStateTransitions)
		if ok {
			cv.WithLabelValues(transition.From, transition.To).Inc()
		}

		instance := instances.Get(transition.Name)
		if instance != nil && transition.To == ec2.InstanceStateTerminated && instance.EC2.TerminationTime != nil {
			duration := instance.EC2.TerminationTime.Sub(instance.ASG.TriggeredAt)
			logger.Infof("instance drainage took %v", duration)

			m, ok := instutil.Histogram(ctx, metricMainLoopDrainDuration)
			if ok {
				m.Observe(duration.Seconds())
			}
		}
	}

	// Log spot state changes
	tcs := instutil.GetTransitionCollector(ctx, metricMainLoopSpotStateTransitions)
	for _, instance := range instances {
		if instance.Spot.RequestID == "" {
			continue
		}

		tcs.Observe(
			instance.InstanceID,
			strings.TrimRight(path.Join(instance.Spot.State, instance.Spot.StatusCode), "/"),
			logutil.FromStruct(instance),
		)
	}
	for _, transition := range tcs.Finish() {
		logger := logutil.Get(ctx).
			WithField(
				"spot-status-transition",
				fmt.Sprintf("%s -> %s", transition.From, transition.To),
			).
			WithFields(transition.Fields)

		if transition.From == "" || transition.To == "" {
			// These transitions are not interesting. We log them in debug
			// level for completeness anyways.
			logger.Debugf("spot request %s changed status: [ %s -> %s ]",
				transition.Name, transition.From, transition.To)
			continue
		}

		logger.Infof("spot request %s changed status: [ %s -> %s ]",
			transition.Name, transition.From, transition.To)

		cv, ok := instutil.CounterVec(ctx, metricMainLoopSpotStateTransitions)
		if ok {
			cv.WithLabelValues(transition.From, transition.To).Inc()
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

func InstMainLoopCordoningInstance(ctx context.Context, instance collectors.Instance) {
	logutil.Get(ctx).
		WithFields(logutil.FromStruct(instance)).
		Info("applying soft taint on instance (cordon)")

	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("soft-taint").Inc()
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
		Warnf("evicting pod %s", pod.Name)

	c, ok := instutil.CounterVec(ctx, metricMainLoopActions)
	if ok {
		c.WithLabelValues("evict-pod").Inc()
	}
}
