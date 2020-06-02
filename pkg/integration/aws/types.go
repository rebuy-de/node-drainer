package aws

import (
	"sort"

	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
)

type Instance struct {
	InstanceID string

	ASG asg.Instance
	EC2 ec2.Instance
}

type Instances []Instance

func (instances Instances) Sort(by By) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return by(&instances[i], &instances[j])
	})

	return instances
}

func (instances Instances) SortReverse(by By) Instances {
	sort.SliceStable(instances, func(i, j int) bool {
		return !by(&instances[i], &instances[j])
	})

	return instances
}

func (instances Instances) Select(selector Selector) Instances {
	result := Instances{}
	for _, i := range instances {
		if selector(&i) {
			result = append(result, i)
		}
	}
	return result
}

func (instances Instances) Filter(selector Selector) Instances {
	result := Instances{}
	for _, i := range instances {
		if !selector(&i) {
			result = append(result, i)
		}
	}
	return result
}
