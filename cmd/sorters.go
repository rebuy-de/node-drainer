package cmd

import (
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
)

const (
	TaintSoft = `node-drainer.rebuy-de.github.io/soft-shutdown`
	TaintHard = `node-drainer.rebuy-de.github.io/hard-shutdown`
)

func SortInstances(instances collectors.Instances) {
	instances.
		Sort(collectors.InstancesByID).
		Sort(collectors.InstancesByLaunchTime).
		Sort(collectors.InstancesByEC2State).
		SortReverse(collectors.InstancesByTriggeredAt)
}

func SortPods(pods collectors.Pods) {
	pods.
		Sort(collectors.PodsByNeedsEviction).
		Sort(collectors.PodsByImmuneToEviction)
}

func InstancesThatNeedLifecycleCompletion() collectors.InstanceSelector {
	return collectors.InstanceQuery().
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Select(collectors.PendingLifecycleCompletion).
		FilterByAllPods(collectors.PodImmuneToEviction)
}

func InstancesThanNeedLifecycleDeletion() collectors.InstanceSelector {
	return collectors.InstanceQuery().
		Filter(collectors.HasEC2Data).
		Select(collectors.HasASGData).
		Filter(collectors.LifecycleDeleted).
		Select(collectors.LifecycleTriggeredOlderThan(time.Hour))
}

func InstancesThatWantShutdown() collectors.InstanceSelector {
	return collectors.InstanceQuery().
		Select(collectors.HasEC2Data).
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Any(
			collectors.InstanceQuery().
				Select(collectors.HasASGData).
				Filter(collectors.LifecycleCompleted),
			collectors.InstanceQuery().
				Select(collectors.HasSpotData).
				Select(collectors.HasSpotStatusCode(
					spot.StatusCodeMarkedForTermination,
					spot.StatusCodeMarkedForStop,
					spot.StatusCodeRequestCanceledAndInstanceRunning,
				)),
		)
}

func InstancesThatNeedCordon() collectors.InstanceSelector {
	return collectors.InstanceQuery().
		Select(InstancesThatWantShutdown()).
		Select(collectors.HasNodeData).
		Filter(collectors.HasTaint(TaintSoft))
}

func InstancesReadyForEviction() collectors.InstanceSelector {
	return collectors.InstanceQuery().
		Select(InstancesThatWantShutdown()).
		Select(collectors.HasTaint(TaintSoft))
}

func PodsThatWantEviction() collectors.PodSelector {
	return collectors.PodQuery().
		SelectByInstance(InstancesReadyForEviction()).
		Filter(collectors.PodImmuneToEviction)
}

func PodsReadyForEviction() collectors.PodSelector {
	return collectors.PodQuery().
		Select(PodsThatWantEviction()).
		Select(collectors.PodCanDecrement)
}

func PodsUnreadyForEviction() collectors.PodSelector {
	return collectors.PodQuery().
		Select(PodsThatWantEviction()).
		Filter(collectors.PodCanDecrement)
}

//func SelectPodsThatAreImminueToEviction(pods collectors.Pods) collectors.Pods {
//	return pods //.Select(collectors.PodImmuneToEviction)
//}
