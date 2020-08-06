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

func (i *Instance) WantsShutdown() bool {
	return i.HasASGData() && i.HasEC2Data() && i.EC2.IsRunning()
}

func (i *Instance) PendingLifecycleCompletion() bool {
	return i.HasASGData() && !i.ASG.Completed
}

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
	all := instance.Pods
	immune, other := all.Split(PodImmuneToEviction)
	can, cannot := other.Split(PodCanDecrement)

	return InstancePodStats{
		Total:            len(all),
		ImmuneToEviction: len(immune),
		CannotDecrement:  len(cannot),
		CanDecrement:     len(can),
	}
}

// Instances is a collection of Instance types with some additional functions.
type Instances []Instance

// Sort returns a sorted list of instances based on the given sorter.
func (instances Instances) Sort(by By) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return by(&instances[i], &instances[j])
	})

	return instances
}

// SortReverse returns a sorted list of instances based on the given sorter.
// The output is reversed.
func (instances Instances) SortReverse(by By) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return !by(&instances[i], &instances[j])
	})

	return instances
}

// Select returns a subset of the instances based on the selector. The subset
// only contains instances, that match the selector.
func (instances Instances) Select(selector Selector) Instances {
	result := Instances{}
	for _, i := range instances {
		if selector(&i) {
			result = append(result, i)
		}
	}
	return result
}

// Filter returns a subset of the instances based on the selector. The subset
// only contains instances, that do not match the selector.
func (instances Instances) Filter(selector Selector) Instances {
	result := Instances{}
	for _, i := range instances {
		if !selector(&i) {
			result = append(result, i)
		}
	}
	return result
}
