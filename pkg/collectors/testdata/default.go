package testdata

import (
	"math"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
)

func Default() collectors.Lists {

	b := NewBuilder()

	b.AddInstance(2, InstanceTemplate{
		EC2:  EC2Running,
		Name: "stateful",
		Node: NodeSchedulable,
	})

	b.AddInstance(2, InstanceTemplate{
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeUnschedulable,
		Name: "stateless",
	})

	b.AddInstance(2, InstanceTemplate{
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeSchedulable,
		Name: "stateless",
	})

	b.AddInstance(2, InstanceTemplate{
		EC2:  EC2Terminated,
		Spot: SpotTerminatedByUser,
		Name: "stateless",
	})

	for _, asgState := range []ASGState{ASGMissing, ASGDone, ASGOnlyCompleted, ASGOnlyDeleted} {
		b.AddInstance(1, InstanceTemplate{
			ASG:  asgState,
			EC2:  EC2Terminated,
			Spot: SpotTerminatedByUser,
			Name: "stateless",
		})
	}

	b.AddInstance(2, InstanceTemplate{
		EC2:  EC2ShuttingDown,
		Spot: SpotRunning,
		Name: "stateless",
	})

	b.AddInstance(1, InstanceTemplate{
		EC2:  EC2Pending,
		Spot: SpotRunning,
		Name: "stateless",
	})

	b.AddInstance(2, InstanceTemplate{
		ASG:  ASGPending,
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeSchedulable,
		Name: "stateless",
	})

	b.AddWorkload(PodTemplate{
		Owner:     OwnerNode,
		Name:      "kube-proxy",
		Namespace: "kube-system",

		TotalReplicas:   math.MaxInt32,
		UnreadyReplicas: 2,
	})

	b.AddWorkload(PodTemplate{
		Owner:     OwnerDaemonSet,
		Name:      "dns",
		Namespace: "kube-system",

		TotalReplicas:   math.MaxInt32,
		UnreadyReplicas: 0,
	})

	return b.Build()
}
