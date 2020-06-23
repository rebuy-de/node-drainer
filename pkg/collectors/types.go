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
