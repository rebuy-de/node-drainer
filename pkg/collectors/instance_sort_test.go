package collectors_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
)

func TestSortInstanceByEC2State(t *testing.T) {
	// contains 4 times every state and is shuffeled manually for the sake of simplicity
	states := []string{"shutting-down", "terminated", "pending", "terminated",
		"stopped", "stopped", "terminated", "shutting-down", "pending",
		"stopping", "shutting-down", "stopped", "running", "running",
		"running", "stopped", "pending", "stopping", "stopping", "running",
		"pending", "stopping", "shutting-down", "terminated"}

	instances := collectors.Instances{}
	for i, state := range states {
		id := fmt.Sprintf("i-%09d", i)
		instances = append(instances, collectors.Instance{
			InstanceID: id,
			EC2: ec2.Instance{
				InstanceID: id,
				State:      state,
			},
		})
	}

	instances.Sort(collectors.InstancesByEC2State)

	statesSorted := []string{}
	for _, instance := range instances {
		statesSorted = append(statesSorted, instance.EC2.State)
	}

	wantedOrder := strings.Split("pending running stopping stopped shutting-down terminated", " ")
	stateCount := 4 // Number of occurences of each state. See 'states' definition.

	for i, state := range statesSorted {
		want := wantedOrder[i/stateCount]
		if state != want {
			t.Errorf("instance %d should have the state %s, but got %s", i, want, state)
		}
	}

	if t.Failed() {
		t.Errorf("got wrong order: %v", statesSorted)
	}
}
