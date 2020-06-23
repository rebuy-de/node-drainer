package collectors

import (
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
)

// CombineInstances merges EC2 instance date from different sources.
func CombineInstances(ai []asg.Instance, ei []ec2.Instance, si []spot.Instance, kn []node.Node) Instances {
	instances := map[string]Instance{}

	for _, i := range ai {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.ID]
		combined.InstanceID = i.ID
		combined.ASG = i
		instances[i.ID] = combined
	}

	for _, i := range ei {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.InstanceID]
		combined.InstanceID = i.InstanceID
		combined.EC2 = i
		instances[i.InstanceID] = combined
	}

	for _, i := range si {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.InstanceID]
		combined.InstanceID = i.InstanceID
		combined.Spot = i
		instances[i.InstanceID] = combined
	}

	for _, n := range kn {
		combined := instances[n.InstanceID]
		combined.InstanceID = n.InstanceID
		combined.Node = n
		instances[n.InstanceID] = combined
	}

	result := Instances{}
	for _, i := range instances {
		result = append(result, i)
	}

	return result
}
