package node

import (
	"context"
	"path"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	informers_v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	TaintEffectNoSchedule = v1.TaintEffectNoSchedule
	TaintEffectNoExecute  = v1.TaintEffectNoExecute
)

type Taint = v1.Taint

type Node struct {
	InstanceID    string  `logfield:"instance-id,omitempty"`
	NodeName      string  `logfield:"node-name,omitempty"`
	Unschedulable bool    `logfield:"node-unschedulable"`
	Taints        []Taint `logfield:"node-taints"`
}

type Client interface {
	// Run executes the EC2 API poller. It will update the instance cache
	// periodically.
	Run(context.Context) error

	// List returns all EC2 Instances that are currently in the cache. Those
	// instance cache will be updated in the background.
	List() []Node

	// SignalEmitter gets triggered every time the cache changes. See syncutil
	// package for more information.
	SignalEmitter() *syncutil.SignalEmitter

	// Healthy indicates whether the background job is running correctly.
	Healthy() bool

	Taint(context.Context, Node, string, v1.TaintEffect) error
}

type client struct {
	kube    kubernetes.Interface
	emitter *syncutil.SignalEmitter

	factory informers.SharedInformerFactory
	nodes   informers_v1.NodeInformer
}

func New(kube kubernetes.Interface) Client {
	factory := informers.NewSharedInformerFactory(kube, 5*time.Second)
	nodes := factory.Core().V1().Nodes()

	return &client{
		kube: kube,

		factory: factory,
		nodes:   nodes,
	}
}

func (c *client) Healthy() bool {
	return c.nodes.Informer().HasSynced()
}

func (c *client) SignalEmitter() *syncutil.SignalEmitter {
	return c.emitter
}

func (c *client) List() []Node {
	result := []Node{}

	list, err := c.nodes.Lister().List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Errorf("lalala")
		return nil
	}

	for _, node := range list {
		result = append(result, Node{
			InstanceID:    path.Base(node.Spec.ProviderID),
			NodeName:      node.ObjectMeta.Name,
			Unschedulable: node.Spec.Unschedulable,
			Taints:        node.Spec.Taints,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		// Sorting by something other than LaunchTime is required, because the
		// time has only second precision and it is quite likely that some
		// instances are started at the same time. And since the list is based
		// on a map, the order would be flaky.
		return result[i].InstanceID < result[j].InstanceID
	})

	//sort.SliceStable(result, func(i, j int) bool {
	//	return result[i].LaunchTime.Before(result[j].LaunchTime)
	//})

	return result
}

func (c *client) Run(ctx context.Context) error {
	// Kubernetes serves an utility to handle API crashes
	defer runtime.HandleCrash()
	c.nodes.Informer().Run(ctx.Done())
	return nil
}

func (c *client) Taint(ctx context.Context, node Node, key string, effect v1.TaintEffect) error {
	if node.NodeName == "" {
		return errors.Errorf("got node with empty name")
	}

	taint := v1.Taint{
		Key:    key,
		Value:  "Exists",
		Effect: effect,
	}

	// We are not getting the node from cache, because it should be as fresh as
	// possible. Also we need to avoid that the append affects the cache.
	upstream, err := c.kube.CoreV1().Nodes().Get(ctx, node.NodeName, meta.GetOptions{})
	if err != nil {
		return errors.WithStack(err)
	}

	upstream.Spec.Taints = append(upstream.Spec.Taints, taint)

	_, err = c.kube.CoreV1().Nodes().Update(ctx, upstream, meta.UpdateOptions{})
	return errors.WithStack(err)
}
