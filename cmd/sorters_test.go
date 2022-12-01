package cmd

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/spot"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/node"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/kube/pod"
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

		result := instances.Select(InstancesThatNeedLifecycleCompletion())
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

		result := instances.Select(InstancesThatNeedLifecycleCompletion())
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

		result := instances.Select(InstancesThatNeedLifecycleCompletion())
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

	result := instances.Select(InstancesThanNeedLifecycleDeletion())
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
	result := instances.Select(InstancesThatWantShutdown())
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

func TestPodSelectors(t *testing.T) {
	const instanceID = "i-00000000000000000"

	var (
		ec2Running = ec2.Instance{
			InstanceID: instanceID,
			State:      "running",
		}
		ec2Terminating = ec2.Instance{
			InstanceID: instanceID,
			State:      "terminating",
		}

		asgC0D0 = asg.Instance{
			ID:        instanceID,
			Completed: false,
			Deleted:   false,
		}
		asgC1D0 = asg.Instance{
			ID:        instanceID,
			Completed: true,
			Deleted:   false,
		}

		podCanDecrement = pod.Pod{
			Name:      "foobar",
			Namespace: "default",
			OwnerKind: "Imaginary",
			OwnerReady: pod.OwnerReadyReason{
				CanDecrement: true,
			},
			PDBReady: pod.PDBReadyReason{
				CanDecrement: true,
			},
		}
		podCannotDecrement = pod.Pod{
			Name:      "foobar",
			Namespace: "default",
			OwnerKind: "Imaginary",
			OwnerReady: pod.OwnerReadyReason{
				CanDecrement: false,
			},
		}

		nodeSoftTaint = node.Node{
			InstanceID: instanceID,
			Taints: []node.Taint{
				node.Taint{
					Key: TaintSoft,
				},
			},
		}
	)

	type wantCase struct {
		evictionWant    bool
		evictionReady   bool
		evictionUnready bool
	}

	cases := []struct {
		name string
		pod  collectors.Pod
		want wantCase
	}{
		{
			name: "empty",
			want: wantCase{
				evictionWant:    false,
				evictionReady:   false,
				evictionUnready: false,
			},
		},
		{
			name: "instance-running",
			want: wantCase{
				evictionWant:    false,
				evictionReady:   false,
				evictionUnready: false,
			},
			pod: collectors.Pod{
				Pod: podCanDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Running,
					Node:       nodeSoftTaint,
				},
			},
		},
		{
			name: "instance-want-shutdown",
			want: wantCase{
				evictionWant:    true,
				evictionReady:   true,
				evictionUnready: false,
			},
			pod: collectors.Pod{
				Pod: podCanDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Running,
					ASG:        asgC0D0,
					Node:       nodeSoftTaint,
				},
			},
		},
		{
			name: "instance-soft-taint-missing",
			want: wantCase{
				evictionWant:    false,
				evictionReady:   false,
				evictionUnready: false,
			},
			pod: collectors.Pod{
				Pod: podCanDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Running,
					ASG:        asgC0D0,
				},
			},
		},
		{
			name: "pod-unready",
			want: wantCase{
				evictionWant:    true,
				evictionReady:   false,
				evictionUnready: true,
			},
			pod: collectors.Pod{
				Pod: podCannotDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Running,
					ASG:        asgC0D0,
					Node:       nodeSoftTaint,
				},
			},
		},
		{
			name: "instance-terminating",
			want: wantCase{
				evictionWant:    false,
				evictionReady:   false,
				evictionUnready: false,
			},
			pod: collectors.Pod{
				Pod: podCanDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Terminating,
					ASG:        asgC0D0,
					Node:       nodeSoftTaint,
				},
			},
		},
		{
			name: "asg-completed",
			want: wantCase{
				evictionWant:    false,
				evictionReady:   false,
				evictionUnready: false,
			},
			pod: collectors.Pod{
				Pod: podCanDecrement,
				Instance: collectors.Instance{
					InstanceID: instanceID,
					EC2:        ec2Running,
					ASG:        asgC1D0,
					Node:       nodeSoftTaint,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				evictionWant    = PodsThatWantEviction()(&tc.pod)
				evictionReady   = PodsReadyForEviction()(&tc.pod)
				evictionUnready = PodsUnreadyForEviction()(&tc.pod)
			)

			// Test expected test cases
			assert.Equal(t,
				tc.want.evictionWant, evictionWant,
				"PodsThatWantEviction should match",
			)
			assert.Equal(t,
				tc.want.evictionReady, evictionReady,
				"PodsReadyForEviction should match",
			)
			assert.Equal(t,
				tc.want.evictionUnready, evictionUnready,
				"PodsUnreadyForEviction should match",
			)

			// Additional sanity checks
			assert.False(t,
				evictionUnready && evictionReady,
				"'ready' and 'unready' should never match on the same pod")
			assert.False(t,
				!evictionWant && evictionReady,
				"'ready' should not match, when 'want' does")
			assert.False(t,
				!evictionWant && evictionUnready,
				"'unready' should not match, when 'want' does")
			assert.False(t,
				evictionWant && !evictionReady && !evictionUnready,
				"either `ready` or `unready` should match, when `want` does")

		})
	}
}

func TestInstanceSelectors(t *testing.T) {
	const (
		instanceID = "i-00000000000000000"
		nodeName   = "ip-172-20-0-1.eu-west-1.compute.internal"
	)

	type wantCase struct {
		wantShutdown bool
	}

	type testCase struct {
		name     string
		instance collectors.Instance
		want     wantCase
	}

	cases := []testCase{
		{
			name: "Empty",
			want: wantCase{wantShutdown: false},
		},
		{
			name: "OnlyEC2",
			want: wantCase{wantShutdown: false},
			instance: collectors.Instance{
				InstanceID: instanceID,
				EC2:        ec2.Instance{InstanceID: instanceID, State: ec2.InstanceStateRunning},
			},
		},
		{
			name: "NewLifecycleHook",
			want: wantCase{wantShutdown: true},
			instance: collectors.Instance{
				InstanceID: instanceID,
				EC2:        ec2.Instance{InstanceID: instanceID, State: ec2.InstanceStateRunning},
				ASG:        asg.Instance{ID: instanceID},
			},
		},
		{
			name: "CompletedLifecycleHook",
			want: wantCase{wantShutdown: false},
			instance: collectors.Instance{
				InstanceID: instanceID,
				EC2:        ec2.Instance{InstanceID: instanceID, State: ec2.InstanceStateRunning},
				ASG:        asg.Instance{ID: instanceID, Completed: true},
			},
		},
	}

	for _, statusCode := range []string{"fulfilled", "instance-terminated-by-user"} {
		cases = append(cases, testCase{
			name: fmt.Sprintf("SpotHealthy(%s)", statusCode),
			want: wantCase{wantShutdown: false},
			instance: collectors.Instance{
				InstanceID: instanceID,
				EC2:        ec2.Instance{InstanceID: instanceID, State: ec2.InstanceStateRunning},
				Spot:       spot.Instance{InstanceID: instanceID, State: "undefined", StatusCode: statusCode},
			},
		})
	}

	for _, statusCode := range []string{"marked-for-termination", "marked-for-stop", "request-canceled-and-instance-running"} {
		cases = append(cases, testCase{
			name: fmt.Sprintf("SpotCancelling(%s)", statusCode),
			want: wantCase{wantShutdown: true},
			instance: collectors.Instance{
				InstanceID: instanceID,
				EC2:        ec2.Instance{InstanceID: instanceID, State: ec2.InstanceStateRunning},
				Spot:       spot.Instance{InstanceID: instanceID, State: "undefined", StatusCode: statusCode},
			},
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				wantShutdown = InstancesThatWantShutdown()(&tc.instance)
			)

			// Test expected test cases
			assert.Equal(t,
				tc.want.wantShutdown, wantShutdown,
				"InstancesThatWantShutdown should match",
			)
		})
	}
}
