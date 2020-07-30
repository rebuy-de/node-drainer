package testdata

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors"

func Default() collectors.Lists {

	b := NewBuilder()

	b.Add(2, Template{
		EC2:  EC2Running,
		Name: "stateful",
		Node: NodeSchedulable,
	})

	b.Add(2, Template{
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeUnschedulable,
		Name: "stateless",
	})

	b.Add(2, Template{
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeSchedulable,
		Name: "stateless",
	})

	b.Add(2, Template{
		EC2:  EC2Terminated,
		Spot: SpotTerminatedByUser,
		Name: "stateless",
	})

	for _, asgState := range []ASGState{ASGMissing, ASGDone, ASGOnlyCompleted, ASGOnlyDeleted} {
		b.Add(1, Template{
			ASG:  asgState,
			EC2:  EC2Terminated,
			Spot: SpotTerminatedByUser,
			Name: "stateless",
		})
	}

	b.Add(2, Template{
		EC2:  EC2ShuttingDown,
		Spot: SpotRunning,
		Name: "stateless",
	})

	b.Add(1, Template{
		EC2:  EC2Pending,
		Spot: SpotRunning,
		Name: "stateless",
	})

	b.Add(2, Template{
		ASG:  ASGPending,
		EC2:  EC2Running,
		Spot: SpotRunning,
		Node: NodeSchedulable,
		Name: "stateless",
	})

	return b.Build()
}
