package cmd

import "github.com/rebuy-de/node-drainer/v2/pkg/collectors"

func SortInstances(instances collectors.Instances) {
	instances.
		Sort(collectors.ByInstanceID).
		Sort(collectors.ByLaunchTime).
		Sort(collectors.ByEC2State).
		SortReverse(collectors.ByTriggeredAt)
}
