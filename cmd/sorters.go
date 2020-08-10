package cmd

import (
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

func SelectInstancesThatNeedLifecycleCompletion(instances collectors.Instances) collectors.Instances {
	return instances.
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Select(collectors.PendingLifecycleCompletion).
		FilterByAllPods(collectors.PodImmuneToEviction)
}

func SelectInstancesThanNeedLifecycleDeletion(instances collectors.Instances) collectors.Instances {
	return instances.
		Filter(collectors.HasEC2Data).
		Filter(collectors.PendingLifecycleCompletion).
		Select(collectors.HasLifecycleMessage)
}

func SelectInstancesThatWantShutdown(instances collectors.Instances) collectors.Instances {
	return instances.
		Select(collectors.HasEC2Data).
		Select(collectors.HasEC2State(ec2.InstanceStateRunning)).
		Select(collectors.PendingLifecycleCompletion)
}

//func SelectPodsThatNeedEviction(pods collectors.Pods) collectors.Pods {
//	return pods
//}
//
//func SelectPodsThatAreImminueToEviction(pods collectors.Pods) collectors.Pods {
//	return pods //.Select(collectors.PodImmuneToEviction)
//}
