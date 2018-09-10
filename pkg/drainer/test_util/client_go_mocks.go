package test_util

import (
	"errors"
	"testing"

	"k8s.io/api/core/v1"
	apiv1beta1 "k8s.io/api/policy/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	policyv1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	rest "k8s.io/client-go/rest"
)

type MockClientset struct {
	fake.Clientset
	PolicyItem *PolicyV1beta1Client
	CoreItem   *MockCoreV1
}

func NewMockClientset() *MockClientset {
	return &MockClientset{}
}

func (mc *MockClientset) SetPolicyV1beta1(p *PolicyV1beta1Client) {
	mc.PolicyItem = p
}

func (mc *MockClientset) SetCore(m *MockCoreV1) {
	mc.CoreItem = m
}

func (mc *MockClientset) PolicyV1beta1() policyv1beta1.PolicyV1beta1Interface {
	return mc.PolicyItem
}

func (mc *MockClientset) CoreV1() corev1.CoreV1Interface {
	return mc.CoreItem
}

type PolicyV1beta1Client struct {
	restClient    rest.Interface
	EvictionsItem *Evictions
}

func NewPolicyV1beta1Client() *PolicyV1beta1Client {
	return &PolicyV1beta1Client{
		EvictionsItem: nil,
	}
}

func (c *PolicyV1beta1Client) Evictions(namespace string) policyv1beta1.EvictionInterface {
	return c.EvictionsItem
}

func (c *PolicyV1beta1Client) PodDisruptionBudgets(namespace string) policyv1beta1.PodDisruptionBudgetInterface {
	return nil
}

func (c *PolicyV1beta1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

type MockCoreV1 struct {
	fakecorev1.FakeCoreV1
	NodesCalled bool
	NodesResult *MockNodes
	PodsCalled  bool
	PodsResult  *MockPods
}

func NewMockCoreV1() *MockCoreV1 {
	return &MockCoreV1{
		NodesCalled: false,
		NodesResult: nil,
		PodsCalled:  false,
		PodsResult:  nil,
	}
}

func (m *MockCoreV1) SetNodes(result *MockNodes) {
	m.NodesResult = result
}

func (m *MockCoreV1) Nodes() corev1.NodeInterface {
	m.NodesCalled = true
	return m.NodesResult
}

func (m *MockCoreV1) SetPods(result *MockPods) {
	m.PodsResult = result
}

func (m *MockCoreV1) Pods(name string) corev1.PodInterface {
	m.PodsCalled = true
	return m.PodsResult
}

type MockNodes struct {
	fakecorev1.FakeNodes
	GetCalled          bool
	UpdateCalled       bool
	GetReturnResult    *v1.Node
	GetReturnError     error
	UpdateReturnResult *v1.Node
	UpdateReturnError  error
}

func NewMockNodes() *MockNodes {
	return &MockNodes{
		GetCalled:          false,
		UpdateCalled:       false,
		GetReturnResult:    nil,
		GetReturnError:     nil,
		UpdateReturnResult: nil,
		UpdateReturnError:  nil,
	}
}

func (m *MockNodes) SetupGet(result *v1.Node, err error) {
	m.GetReturnResult = result
	m.GetReturnError = err
}

func (m *MockNodes) SetupUpdate(result *v1.Node, err error) {
	m.UpdateReturnResult = result
	m.UpdateReturnError = err
}

func (m *MockNodes) Get(name string, options meta_v1.GetOptions) (result *v1.Node, err error) {
	m.GetCalled = true
	result = m.GetReturnResult
	err = m.GetReturnError
	return
}

func (m *MockNodes) Update(node *v1.Node) (result *v1.Node, err error) {
	m.UpdateCalled = true
	result = m.UpdateReturnResult
	err = m.UpdateReturnError
	return
}

type MockPods struct {
	fakecorev1.FakePods
	ListCalled       bool
	ListReturnResult *v1.PodList
	ListReturnError  error
}

func NewMockPods() *MockPods {
	return &MockPods{
		ListCalled:       false,
		ListReturnResult: nil,
		ListReturnError:  nil,
	}
}

func (m *MockPods) SetupList(result *v1.PodList, err error) {
	m.ListReturnResult = result
	m.ListReturnError = err
}

func (m *MockPods) List(opts meta_v1.ListOptions) (result *v1.PodList, err error) {
	m.ListCalled = true
	result = m.ListReturnResult
	err = m.ListReturnError
	return
}

type Evictions struct {
	client       rest.Interface
	ns           string
	Tries        int
	EvictSuccess []bool
	EvictCalled  bool
	T            *testing.T
}

func NewEvictions(c *PolicyV1beta1Client, namespace string, t *testing.T) *Evictions {
	return &Evictions{
		client:       c.RESTClient(),
		ns:           namespace,
		Tries:        0,
		T:            t,
		EvictCalled:  false,
		EvictSuccess: []bool{false},
	}
}

func (c *Evictions) Evict(eviction *apiv1beta1.Eviction) error {
	c.EvictCalled = true

	if c.Tries >= len(c.EvictSuccess) {
		c.T.Fail()
	}

	if c.EvictSuccess[c.Tries] == true {
		c.Tries++
		return nil
	}

	c.Tries++
	return errors.New("")
}
