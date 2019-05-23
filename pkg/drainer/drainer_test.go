package drainer

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	tu "github.com/rebuy-de/node-drainer/pkg/drainer/test_util"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDrain(t *testing.T) {
	clientset, _, _, _, nodes := tu.GenerateMocks()
	drainer := NewDrainer(clientset)

	nodeSpec := v1.NodeSpec{Taints: []v1.Taint{v1.Taint{}}}
	node := &v1.Node{Spec: nodeSpec}
	node.SetName("matching")

	cases := []struct {
		name       string
		nodeName   string
		nodeReturn *v1.Node
		want       bool
	}{
		{
			name:       "empty",
			nodeName:   "",
			nodeReturn: nil,
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes.SetupGet(tc.nodeReturn, nil)
			_, err := drainer.Drain(tc.nodeName)
			var have bool
			if err == nil {
				have = true
			} else {
				have = false
			}
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestNode(t *testing.T) {
	clientset, _, _, _, nodes := tu.GenerateMocks()
	drainer := NewDrainer(clientset)
	node := &v1.Node{}

	cases := []struct {
		name      string
		nodeName  string
		returnVal *v1.Node
		returnErr error
		want      *v1.Node
	}{
		{
			name:      "name_missing",
			nodeName:  "",
			returnVal: nil,
			returnErr: nil,
			want:      nil,
		},
		{
			name:      "working",
			nodeName:  "name",
			returnVal: node,
			returnErr: nil,
			want:      node,
		},
		{
			name:      "status_error",
			nodeName:  "name",
			returnVal: nil,
			returnErr: &errors.StatusError{},
			want:      nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes.SetupGet(tc.returnVal, tc.returnErr)
			have := drainer.node(tc.nodeName)
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestHasShutdownTaint(t *testing.T) {
	clientset := tu.NewMockClientset()
	drainer := NewDrainer(clientset)

	tInitial := v1.Taint{
		Key:    "rebuy.com/initial",
		Value:  "Exists",
		Effect: "NoExecute",
	}

	tShutdown := v1.Taint{
		Key:    "rebuy.com/shutdown",
		Value:  "Exists",
		Effect: "NoSchedule",
	}

	cases := []struct {
		name   string
		taints []v1.Taint
		want   bool
	}{
		{
			name:   "empty",
			taints: []v1.Taint{},
			want:   false,
		},
		{
			name:   "has_taint",
			taints: []v1.Taint{tInitial, tShutdown},
			want:   true,
		},
		{
			name:   "doesnt_have_taint",
			taints: []v1.Taint{tInitial},
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			have := drainer.hasShutdownTaint(tc.taints)
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestEvictAllPods(t *testing.T) {
	resultOne := tu.GeneratePodList(1, "matching", 0)
	resultMany := tu.GeneratePodList(4, "matching", 0)
	resultNone := tu.GeneratePodList(2, "nonmatching", 0)

	node := v1.Node{}
	node.SetName("matching")

	cases := []struct {
		name            string
		node            *v1.Node
		podList         *v1.PodList
		evictionSuccess []bool
		err             error
		wantEvictCalled bool
	}{
		{
			name:            "one_matching",
			node:            &node,
			podList:         resultOne,
			evictionSuccess: []bool{true},
			err:             nil,
			wantEvictCalled: true,
		},
		{
			name:            "multiple_matching",
			node:            &node,
			podList:         resultMany,
			evictionSuccess: []bool{true, true, true, true},
			err:             nil,
			wantEvictCalled: true,
		},
		{
			name:            "none_matching",
			node:            &node,
			podList:         resultNone,
			evictionSuccess: []bool{},
			err:             nil,
			wantEvictCalled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientset, policy, _, pods, _ := tu.GenerateMocks()
			drainer := NewDrainer(clientset)
			drainer.Wait = false

			evictions := tu.NewEvictions(policy, "default", t)
			policy.EvictionsItem = evictions

			evictions.EvictSuccess = tc.evictionSuccess
			pods.SetupList(tc.podList, tc.err)
			drainer.evictAllPods(tc.node)
		})
	}
}

func TestGetRemainingPodCountCrash(t *testing.T) {
	if os.Getenv("TEST_GetRemainingPodCountCRASH") == "crash" {
		clientset, _, _, pods, _ := tu.GenerateMocks()
		drainer := NewDrainer(clientset)

		pods.SetupList(nil, fmt.Errorf("New Error"))
		drainer.getRemainingPodCount(&v1.Node{})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGetRemainingPodCountCrash")
	cmd.Env = append(os.Environ(), "TEST_GetRemainingPodCountCRASH=crash")
	err := cmd.Run()
	var have bool
	if err == nil {
		have = true
	} else {
		have = false
	}
	if have != false {
		t.Fail()
	}
}

func TestGetRemainingPodCount(t *testing.T) {
	clientset, _, _, pods, _ := tu.GenerateMocks()
	drainer := NewDrainer(clientset)

	resultOne := tu.GeneratePodList(1, "matching", 0)
	resultMany := tu.GeneratePodList(4, "matching", 0)
	resultNone := tu.GeneratePodList(2, "nonmatching", 0)
	resultSome := tu.GeneratePodList(2, "matching", 1)

	node := v1.Node{}
	node.SetName("matching")

	cases := []struct {
		name    string
		node    *v1.Node
		podList *v1.PodList
		err     error
		want    int
	}{
		{
			name:    "one_matching",
			node:    &node,
			podList: resultOne,
			err:     nil,
			want:    1,
		},
		{
			name:    "multiple_matching",
			node:    &node,
			podList: resultMany,
			err:     nil,
			want:    4,
		},
		{
			name:    "none_matching",
			node:    &node,
			podList: resultNone,
			err:     nil,
			want:    0,
		},
		{
			name:    "some_with_toleration",
			node:    &node,
			podList: resultSome,
			err:     nil,
			want:    1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pods.SetupList(tc.podList, tc.err)
			have := drainer.getRemainingPodCount(tc.node)
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestEvict(t *testing.T) {
	clientset, policy, _, _, _ := tu.GenerateMocks()
	drainer := NewDrainer(clientset)
	drainer.Wait = false
	evictions := tu.NewEvictions(policy, "default", t)
	policy.EvictionsItem = evictions

	cases := []struct {
		name      string
		successes []bool
		want      bool
	}{
		{
			name:      "instant_success",
			successes: []bool{true},
			want:      true,
		},
		{
			name:      "delayed_success",
			successes: []bool{false, false, false, true},
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evictions.Tries = 0
			evictions.EvictCalled = false
			evictions.EvictSuccess = tc.successes
			drainer.evict(corev1.Pod{})
			have := evictions.EvictCalled
			if have != tc.want {
				t.Fail()
			}
		})
	}
}

func TestPodHasInitToleration(t *testing.T) {
	drainer := NewDrainer(fake.NewSimpleClientset())
	drainer.Wait = false

	matchingToleration := v1.Toleration{
		Effect:   v1.TaintEffectNoExecute,
		Key:      "rebuy.com/initial",
		Operator: v1.TolerationOpExists,
	}
	nonMatchingToleration := v1.Toleration{
		Effect:   v1.TaintEffectNoSchedule,
		Key:      "rebuy.com/random",
		Operator: v1.TolerationOpExists,
	}

	tolerations := []v1.Toleration{matchingToleration}
	tolerations2 := []v1.Toleration{nonMatchingToleration}
	tolerations3 := []v1.Toleration{matchingToleration, nonMatchingToleration}
	tolerations4 := []v1.Toleration{nonMatchingToleration, nonMatchingToleration}

	cases := []struct {
		name   string
		taints []v1.Toleration
		want   bool
	}{
		{
			name:   "matching_tolerations",
			taints: tolerations,
			want:   true,
		},
		{
			name:   "non-matching_tolerations",
			taints: tolerations2,
			want:   false,
		},
		{
			name:   "empty_tolerations",
			taints: []v1.Toleration{v1.Toleration{}},
			want:   false,
		},
		{
			name:   "matching_list_tolerations",
			taints: tolerations3,
			want:   true,
		},
		{
			name:   "non-matching_list_tolerations",
			taints: tolerations4,
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			have := drainer.podHasInitToleration(tc.taints)
			if have != tc.want {
				t.Fail()
			}
		})
	}
}
