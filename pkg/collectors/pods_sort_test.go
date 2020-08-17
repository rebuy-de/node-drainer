package collectors_test

import (
	"testing"

	"github.com/rebuy-de/node-drainer/v2/pkg/collectors"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/asg"
	"github.com/rebuy-de/node-drainer/v2/pkg/collectors/aws/ec2"
)

func TestSortPodByNeedsEviction(t *testing.T) {
	podsWithEvictionNotNeeded := collectors.Pod{
		Instance: collectors.Instance{
			InstanceID: "xxx",
			EC2: ec2.Instance{
				InstanceID: "xxx",
				State:      "running",
			},
		},
	}

	podsWithEvictionNeeded := podsWithEvictionNotNeeded
	podsWithEvictionNeeded.Instance.ASG = asg.Instance{
		ID: "xxx",
	}

	if !podsWithEvictionNeeded.WantsShutdown() {
		t.Fatal("sanity check failed. pods should want shutdown")
	}
	if podsWithEvictionNotNeeded.WantsShutdown() {
		t.Fatal("sanity check failed. pods should not want shutdown")
	}

	pods := collectors.Pods{
		podsWithEvictionNotNeeded,
		podsWithEvictionNeeded,
		podsWithEvictionNotNeeded,
		podsWithEvictionNeeded,
	}

	pods.Sort(collectors.PodsByNeedsEviction)

	if !pods[0].NeedsEviction() {
		t.Error("pod 0 should need eviction")
	}
	if !pods[1].NeedsEviction() {
		t.Error("pod 1 should need eviction")
	}
	if pods[2].NeedsEviction() {
		t.Error("pod 2 should not need eviction")
	}
	if pods[3].NeedsEviction() {
		t.Error("pod 3 should not need eviction")
	}
}
