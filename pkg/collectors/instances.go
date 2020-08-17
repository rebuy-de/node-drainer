package collectors

import (
	"sort"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
)

// Instance is the combined data from different sources.
type Instance struct {
	InstanceID string `logfield:"instance-id"`

	ASG  asg.Instance  `logfield:",squash"`
	EC2  ec2.Instance  `logfield:",squash"`
	Spot spot.Instance `logfield:",squash"`
	Node node.Node     `logfield:",squash"`

	Pods Pods `logfield:"-"`
}

func (i *Instance) HasEC2Data() bool {
	return i != nil && i.EC2.InstanceID != ""
}

func (i *Instance) HasASGData() bool {
	return i != nil && i.ASG.ID != ""
}

// Deprecated: Should use filters instead.
func (i *Instance) WantsShutdown() bool {
	return i.HasASGData() && i.HasEC2Data() && i.EC2.IsRunning()
}

// Deprecated: Should use filters instead.
func (i *Instance) PendingLifecycleCompletion() bool {
	return i.HasASGData() && !i.ASG.Completed
}

// Deprecated: Should use filters instead.
func (i *Instance) HasLifecycleMessage() bool {
	return i.HasASGData() && !i.ASG.Deleted
}

func (i *Instance) HasEC2State(states ...string) bool {
	for _, state := range states {
		if i.EC2.State == state {
			return true
		}
	}

	return false
}

type InstancePodStats struct {
	Total            int
	ImmuneToEviction int
	CannotDecrement  int
	CanDecrement     int
}

func (instance Instance) PodStats() InstancePodStats {
	var (
		podImmuneToEviction = PodQuery().
					Select(PodImmuneToEviction)
		podCanDecrement = PodQuery().
				Filter(PodImmuneToEviction).
				Select(PodCanDecrement)
		podCannotDecrement = PodQuery().
					Filter(PodImmuneToEviction).
					Filter(PodCanDecrement)
	)

	var (
		all    = instance.Pods
		immune = instance.Pods.Select(podImmuneToEviction)
		can    = instance.Pods.Select(podCanDecrement)
		cannot = instance.Pods.Select(podCannotDecrement)
	)

	return InstancePodStats{
		Total:            len(all),
		ImmuneToEviction: len(immune),
		CannotDecrement:  len(cannot),
		CanDecrement:     len(can),
	}
}

// Instances is a collection of Instance types with some additional functions.
type Instances []Instance

func (instances Instances) Get(instanceID string) *Instance {
	for _, instance := range instances {
		if instance.InstanceID == instanceID {
			return &instance
		}
	}
	return nil
}

// Sort returns a sorted list of instances based on the given sorter.
func (instances Instances) Sort(by InstancesBy) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return by(&instances[i], &instances[j])
	})

	return instances
}

// SortReverse returns a sorted list of instances based on the given sorter.
// The output is reversed.
func (instances Instances) SortReverse(by InstancesBy) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return !by(&instances[i], &instances[j])
	})

	return instances
}

// Select returns a subset of the instances based on the selector. The subset
// only contains instances, that match the selector.
func (instances Instances) Select(selector InstanceSelector) Instances {
	result := Instances{}
	for _, i := range instances {
		if selector(&i) {
			result = append(result, i)
		}
	}
	return result
}
