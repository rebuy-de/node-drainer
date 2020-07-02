package collectors

import (
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

// Combine merges all data sources and returns all in one structs for Pods and
// Instances.
func Combine(l Lists) (Instances, Pods) {
	instances := combineInstances(l)
	pods := combinePods(instances, l.Pods)

	for _, pod := range pods {
		instance, found := instances[pod.Instance.InstanceID]
		if !found {
			continue
		}

		instance.Pods = append(instance.Pods, pod)

		instances[pod.Instance.InstanceID] = instance
	}

	result := Instances{}
	for _, i := range instances {
		result = append(result, i)
	}

	return result, pods
}

// combineInstances merges EC2 instance date from different sources.
func combineInstances(l Lists) map[string]Instance {
	instances := map[string]Instance{}

	for _, i := range l.ASG {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.ID]
		combined.InstanceID = i.ID
		combined.ASG = i
		instances[i.ID] = combined
	}

	for _, i := range l.EC2 {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.InstanceID]
		combined.InstanceID = i.InstanceID
		combined.EC2 = i
		instances[i.InstanceID] = combined
	}

	for _, i := range l.Spot {
		// It returns the empty value, if the key does not exist yet. Therefore
		// we do not need any checks whether the instances is already in the
		// map and just need to set the Instance ID.
		combined := instances[i.InstanceID]
		combined.InstanceID = i.InstanceID
		combined.Spot = i
		instances[i.InstanceID] = combined
	}

	for _, n := range l.Nodes {
		combined := instances[n.InstanceID]
		combined.InstanceID = n.InstanceID
		combined.Node = n
		instances[n.InstanceID] = combined
	}

	return instances
}

// combinePods merges Pod data with instance data.
func combinePods(instances map[string]Instance, pods []pod.Pod) Pods {
	nodes := map[string]Instance{}
	for _, i := range instances {
		nn := i.NodeName()
		if nn == "" {
			continue
		}

		nodes[nn] = i
	}

	result := []Pod{}
	for _, pod := range pods {
		instance, found := nodes[pod.NodeName]
		if !found {
			continue
		}

		result = append(result, Pod{
			Instance: instance,
			Pod:      pod,
		})
	}

	return Pods(result)
}
