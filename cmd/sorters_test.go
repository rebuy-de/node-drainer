package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/testdata"
)

func TestSelectInstancesThatNeedLifecycleCompletion(t *testing.T) {
	b := testdata.NewBuilder()

	for _, ec2State := range testdata.AllEC2States {
		for _, asgState := range testdata.AllASGStates {
			b.AddInstance(1, testdata.InstanceTemplate{
				ASG:  asgState,
				EC2:  ec2State,
				Spot: testdata.SpotRunning,
				Node: testdata.NodeSchedulable,
				Name: fmt.Sprintf("%v_%v", ec2State, asgState),
			})
		}
	}

	instances, _ := collectors.Combine(b.Build())

	result := SelectInstancesThatNeedLifecycleCompletion(instances)
	assert.Len(t, result, 2)
	names := []string{}

	for _, instance := range result {
		assert.Equal(t, instance.InstanceID, instance.EC2.InstanceID, "should have EC2 data")
		assert.Equal(t, instance.InstanceID, instance.ASG.ID, "should have ASG data")
		assert.Equal(t, instance.EC2.State, "running", "should be in running state")
		assert.False(t, instance.ASG.Completed, "should not be completed yet")

		names = append(names, instance.EC2.InstanceName)
	}

	assert.Contains(t, names, "running_only-deleted")
	assert.Contains(t, names, "running_pending")
}

func TestSelectInstancesThatNeedLifecycleDeletion(t *testing.T) {
	b := testdata.NewBuilder()

	for _, ec2State := range testdata.AllEC2States {
		for _, asgState := range testdata.AllASGStates {
			b.AddInstance(1, testdata.InstanceTemplate{
				ASG:  asgState,
				EC2:  ec2State,
				Spot: testdata.SpotRunning,
				Node: testdata.NodeSchedulable,
				Name: fmt.Sprintf("%v_%v", ec2State, asgState),
			})
		}
	}

	instances, _ := collectors.Combine(b.Build())

	result := SelectInstancesThanNeedLifecycleDeletion(instances)
	assert.Len(t, result, 1)

	for _, instance := range result {
		assert.Equal(t, instance.EC2.InstanceID, "", "should not have EC2 data")
		assert.True(t, instance.ASG.Completed, "should be completed")
		assert.False(t, instance.ASG.Deleted, "should not be deleted")
	}
}
