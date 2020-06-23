package collectors

import (
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
)

type Pod struct {
	Instance
	pod.Pod
}

type Pods []Pod
