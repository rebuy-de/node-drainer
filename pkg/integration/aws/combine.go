package aws

import (
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/integration/aws/ec2"
)

// CombineInstances merges EC2 instance date from different sources.
func CombineInstances(ai []asg.Instance, ei []ec2.Instance) Instances {
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

	result := Instances{}
	for _, i := range instances {
		result = append(result, i)
	}

	return result
}
