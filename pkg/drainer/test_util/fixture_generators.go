package test_util

import "k8s.io/api/core/v1"

func GeneratePodList(count int, nodeName string, countWithTolerations int) *v1.PodList {
	matchingToleration := v1.Toleration{
		Effect:   v1.TaintEffectNoExecute,
		Key:      "rebuy.com/initial",
		Operator: v1.TolerationOpExists,
	}
	tolerationList := []v1.Toleration{matchingToleration}

	spec := v1.PodSpec{NodeName: nodeName, Tolerations: []v1.Toleration{}}
	pod := v1.Pod{Spec: spec}
	podList := v1.PodList{Items: []v1.Pod{}}

	for i := 0; i < count; i++ {
		podList.Items = append(podList.Items, pod)
		if i < countWithTolerations {
			podList.Items[len(podList.Items)-1].Spec.Tolerations = tolerationList
		}
	}

	return &podList
}

func GenerateMocks() (clientset *MockClientset, policy *PolicyV1beta1Client, core *MockCoreV1, pods *MockPods, nodes *MockNodes) {
	clientset = NewMockClientset()
	policy = NewPolicyV1beta1Client()
	core = NewMockCoreV1()
	pods = NewMockPods()
	nodes = NewMockNodes()
	clientset.SetPolicyV1beta1(policy)
	clientset.SetCore(core)
	core.SetPods(pods)
	core.SetNodes(nodes)
	return
}
