package collectors

import (
	"sort"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

type Pod struct {
	Instance `logfield:",squash"`
	pod.Pod  `logfield:",squash"`
}

// Deprecated: Should use filters instead.
func (p *Pod) NeedsEviction() bool {
	if p == nil {
		return false
	}

	if p.Pod.ImmuneToEviction() {
		return false
	}

	return p.Instance.WantsShutdown()
}

// NodeName returns the NodeName which it tries to get from Kubernetes or EC2
// data. Returns an empty string, if the NodeName could not been determinated.
func (i *Instance) NodeName() string {
	if i == nil {
		return ""
	}

	if i.Node.NodeName != "" {
		return i.Node.NodeName
	}

	if i.EC2.NodeName != "" {
		return i.EC2.NodeName
	}

	return ""
}

type Pods []Pod

// Sort returns a sorted list of pods based on the given sorter.
func (pods Pods) Sort(by PodsBy) Pods {
	sort.SliceStable(pods, func(i, j int) bool {
		return by(&pods[i], &pods[j])
	})

	return pods
}

func (pods Pods) Select(selector PodSelector) Pods {
	result := Pods{}

	for _, p := range pods {
		if selector(&p) {
			result = append(result, p)
		}
	}

	return result
}
