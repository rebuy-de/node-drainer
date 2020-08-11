package cmd

import (
	"time"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
)

func SortInstances(instances collectors.Instances) {
	instances.
		Sort(collectors.ByInstanceID).
		Sort(collectors.ByLaunchTime).
		Sort(collectors.ByEC2State).
		SortReverse(collectors.ByTriggeredAt)
}

func SortPods(pods collectors.Pods) {
	pods.
		Sort(collectors.PodsByNeedsEviction).
		Sort(collectors.PodsByImmuneToEviction)
}

func InstancesThatNeedLifecycleCompletion() collectors.Selector {
	return collectors.InstanceQuery().
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Select(collectors.PendingLifecycleCompletion).
		FilterByAllPods(collectors.PodImmuneToEviction)
}

func InstancesThanNeedLifecycleDeletion() collectors.Selector {
	return collectors.InstanceQuery().
		Filter(collectors.HasEC2Data).
		Select(collectors.HasASGData).
		Filter(collectors.LifecycleDeleted).
		Select(collectors.LifecycleTriggeredOlderThan(time.Hour))
}

func InstancesThatWantShutdown() collectors.Selector {
	return collectors.InstanceQuery().
		Select(collectors.HasEC2Data).
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Select(collectors.PendingLifecycleCompletion)
}

func PodsThatNeedEviction() collectors.PodSelector {
	return collectors.PodQuery().
		SelectByInstance(InstancesThanNeedLifecycleDeletion()).
		Filter(collectors.PodImmuneToEviction)
}

func PodsReadyForEviction() collectors.PodSelector {
	return collectors.PodQuery().
		SelectByInstance(InstancesThanNeedLifecycleDeletion()).
		Filter(collectors.PodImmuneToEviction).
		Select(collectors.PodCanDecrement)
}

//func SelectPodsThatAreImminueToEviction(pods collectors.Pods) collectors.Pods {
//	return pods //.Select(collectors.PodImmuneToEviction)
//}
