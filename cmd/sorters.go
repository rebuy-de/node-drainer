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
		Select(collectors.PendingLifecycleCompletion)
}

func SelectInstancesThanNeedLifecycleDeletion(instances collectors.Instances) collectors.Instances {
	return instances.
		Filter(collectors.HasEC2Data).
		Filter(collectors.PendingLifecycleCompletion).
		Select(collectors.HasLifecycleMessage)
}
