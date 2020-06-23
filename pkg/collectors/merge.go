package collectors

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"

func Merge(instances Instances, pods []pod.Pod) Pods {
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
