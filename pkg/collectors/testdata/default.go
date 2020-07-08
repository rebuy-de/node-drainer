package testdata

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors"

func Default() collectors.Lists {

	b := NewBuilder()

	b.Add(2, Template{
		EC2:  EC2Running,
		Name: "stateful",
	})

	b.Add(2, Template{
		EC2:  EC2Running,
		Spot: SpotRunning,
		Name: "stateless",
	})

	b.Add(2, Template{
		EC2:  EC2Terminated,
		Spot: SpotTerminatedByUser,
		Name: "stateless",
	})

	b.Add(2, Template{
		EC2:  EC2Terminated,
		Spot: SpotTerminatedByUser,
		Name: "stateless",
	})

	return b.Build()
}
