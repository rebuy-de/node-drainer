package pod

import (
	"context"
	"sort"
	"time"

	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	informers_v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Pod struct {
	Name      string `logfield:"pod-name"`
	Namespace string `logfield:"pod-namespace"`
	NodeName  string `logfield:"node-name"`

	AppName      string `logfield:"-"`
	AppInstance  string `logfield:"-"`
	AppVersion   string `logfield:"-"`
	AppComponent string `logfield:"-"`

	Ready       bool      `logfield:"pod-ready"`
	CreatedTime time.Time `logfield:"pod-created-time"`
}

type Client interface {
	// Run executes the EC2 API poller. It will update the instance cache
	// periodically.
	Run(context.Context) error

	// List returns all EC2 Instances that are currently in the cache. Those
	// instance cache will be updated in the background.
	List() []Pod

	// SignalEmitter gets triggered every time the cache changes. See syncutil
	// package for more information.
	SignalEmitter() *syncutil.SignalEmitter

	// Healthy indicates whether the background job is running correctly.
	Healthy() bool
}

type client struct {
	kube    kubernetes.Interface
	cache   map[string]Pod
	emitter *syncutil.SignalEmitter

	factory informers.SharedInformerFactory
	pods    informers_v1.PodInformer
}

func New(kube kubernetes.Interface) Client {
	factory := informers.NewSharedInformerFactory(kube, 5*time.Second)
	pods := factory.Core().V1().Pods()

	return &client{
		kube: kube,

		factory: factory,
		pods:    pods,
	}
}

func (c *client) Healthy() bool {
	return c.pods.Informer().HasSynced()
}

func (c *client) SignalEmitter() *syncutil.SignalEmitter {
	return c.emitter
}

func (c *client) List() []Pod {
	result := []Pod{}

	list, err := c.pods.Lister().List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Errorf("unexpected error")
		return nil
	}

	for _, pod := range list {
		labels := pod.ObjectMeta.Labels
		if labels == nil {
			// empty map, so retrieving a key fails silently
			labels = map[string]string{}
		}

		ready := true
		for _, condition := range pod.Status.Conditions {
			if condition.Status != v1.ConditionTrue {
				ready = false
				break
			}
		}

		result = append(result, Pod{
			Name:      pod.ObjectMeta.Name,
			Namespace: pod.ObjectMeta.Namespace,
			NodeName:  pod.Spec.NodeName,

			Ready:       ready,
			CreatedTime: pod.ObjectMeta.CreationTimestamp.Time,

			AppName:      labels["app.kubernetes.io/name"],
			AppInstance:  labels["app.kubernetes.io/instance"],
			AppVersion:   labels["app.kubernetes.io/version"],
			AppComponent: labels["app.kubernetes.io/component"],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		// Sorting by something other than CreatedTime is required, because the
		// time has only second precision and it is quite likely that some
		// instances are started at the same time. And since the list is based
		// on a map, the order would be flaky.
		return result[i].Name < result[j].Name
	})

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedTime.Before(result[j].CreatedTime)
	})

	return result
}

func (c *client) Run(ctx context.Context) error {
	// Kubernetes serves an utility to handle API crashes
	defer runtime.HandleCrash()
	c.pods.Informer().Run(ctx.Done())
	return nil
}
