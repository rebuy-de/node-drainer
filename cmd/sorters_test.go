package cmd

import (
	"fmt"
	"math"
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

	t.Run("StatesOnEmptyInstances", func(t *testing.T) {
		instances, _ := collectors.Combine(b.Build())

		result := SelectInstancesThatNeedLifecycleCompletion(instances)
		assert.Len(t, result, 2)
		names := []string{}

		for _, instance := range result {
			assert.Equal(t, instance.InstanceID, instance.EC2.InstanceID, "should have EC2 data")
			assert.Equal(t, instance.InstanceID, instance.ASG.ID, "should have ASG data")
			assert.Equal(t, instance.EC2.State, "running", "should be in running state")
			assert.False(t, instance.ASG.Completed, "should not be completed yet")

			for _, pod := range instance.Pods {
				assert.True(t, pod.ImmuneToEviction(), "there should only be immune pods left")
				assert.NotContains(t, []string{"Deployment", "StatefulSet", "Job", "ReplicaSet"},
					pod.OwnerKind, "only DaemonSets and Nodes are allowed")
				assert.False(t, pod.OwnerReady.CanDecrement, "immune pods are not ready for decrement")
			}

			names = append(names, instance.EC2.InstanceName)
		}

		assert.Contains(t, names, "running_only-deleted")
		assert.Contains(t, names, "running_pending")
	})

	t.Run("OnlyImmunePods", func(t *testing.T) {
		instances, _ := collectors.Combine(b.Build())
		b.AddWorkload(testdata.PodTemplate{
			Owner: testdata.OwnerDaemonSet,
			Name:  "dns",

			TotalReplicas:   math.MaxInt32,
			UnreadyReplicas: 0,
		})

		result := SelectInstancesThatNeedLifecycleCompletion(instances)
		assert.Len(t, result, 2)
		names := []string{}

		for _, instance := range result {
			for _, pod := range instance.Pods {
				assert.True(t, pod.ImmuneToEviction(), "there should only be immune pods left")
				assert.NotContains(t, []string{"Deployment", "StatefulSet", "Job", "ReplicaSet"},
					pod.OwnerKind, "only DaemonSets and Nodes are allowed")
				assert.False(t, pod.OwnerReady.CanDecrement, "immune pods are not ready for decrement")
			}

			names = append(names, instance.EC2.InstanceName)
		}

		assert.Contains(t, names, "running_only-deleted")
		assert.Contains(t, names, "running_pending")
	})

	t.Run("NoCompletionWithPods", func(t *testing.T) {
		b.AddWorkload(testdata.PodTemplate{
			Owner: testdata.OwnerDeployment,
			Name:  "worker",

			TotalReplicas:   math.MaxInt32,
			UnreadyReplicas: 0,
		})

		instances, _ := collectors.Combine(b.Build())

		result := SelectInstancesThatNeedLifecycleCompletion(instances)
		assert.Len(t, result, 0)
	})
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
	assert.Len(t, result, 2)

	for _, instance := range result {
		assert.Equal(t, instance.EC2.InstanceID, "", "should not have EC2 data")
		assert.Equal(t, instance.ASG.ID, instance.InstanceID, "should have ASG data")
		assert.False(t, instance.ASG.Deleted, "should not be deleted")
	}
}

func TestSelectInstancesThatWantShutdown(t *testing.T) {
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
	result := SelectInstancesThatWantShutdown(instances)
	assert.Len(t, result, 2)
	names := []string{}

	for _, instance := range result {
		assert.Equal(t, instance.InstanceID, instance.EC2.InstanceID, "should have EC2 data")
		assert.Equal(t, instance.InstanceID, instance.ASG.ID, "should have ASG data")
		assert.Equal(t, instance.EC2.State, "running", "should be in running state")
		assert.False(t, instance.ASG.Completed, "should not be completed yet")

		names = append(names, instance.EC2.InstanceName)
	}

	assert.Contains(t, names, "running_pending")
	assert.Contains(t, names, "running_only-deleted")
}
