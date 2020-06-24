package pod

import (
	"context"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/syncutil"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	apps_informers "k8s.io/client-go/informers/apps/v1"
	core_informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type Pod struct {
	Name      string `logfield:"pod-name"`
	Namespace string `logfield:"pod-namespace"`
	NodeName  string `logfield:"node-name"`

	AppName      string `logfield:"-"`
	AppInstance  string `logfield:"-"`
	AppVersion   string `logfield:"-"`
	AppComponent string `logfield:"-"`

	OwnerKind   string           `logfield:"pod-owner-kind"`
	OwnerName   string           `logfield:"pod-owner-name"`
	OwnerReady  OwnerReadyReason `logfield:",squash"`
	Ready       bool             `logfield:"pod-ready"`
	CreatedTime time.Time        `logfield:"pod-created-time"`
}

func (p *Pod) ImmuneToEviction() bool {
	if p == nil {
		return true
	}

	return p.OwnerKind == "DaemonSet" || p.OwnerKind == "Node"
}

type Client interface {
	// Run executes the EC2 API poller. It will update the instance cache
	// periodically.
	Run(context.Context) error

	// List returns all EC2 Instances that are currently in the cache. Those
	// instance cache will be updated in the background.
	List(context.Context) []Pod

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

	pods   core_informers.PodInformer
	rs     apps_informers.ReplicaSetInformer
	sts    apps_informers.StatefulSetInformer
	deploy apps_informers.DeploymentInformer
}

func New(kube kubernetes.Interface) Client {
	return &client{
		kube: kube,

		pods:   informers.NewSharedInformerFactory(kube, 5*time.Second).Core().V1().Pods(),
		rs:     informers.NewSharedInformerFactory(kube, 5*time.Second).Apps().V1().ReplicaSets(),
		sts:    informers.NewSharedInformerFactory(kube, 5*time.Second).Apps().V1().StatefulSets(),
		deploy: informers.NewSharedInformerFactory(kube, 5*time.Second).Apps().V1().Deployments(),
	}
}

func (c *client) Healthy() bool {
	return c.pods.Informer().HasSynced() &&
		c.rs.Informer().HasSynced() &&
		c.sts.Informer().HasSynced() &&
		c.deploy.Informer().HasSynced()
}

func (c *client) SignalEmitter() *syncutil.SignalEmitter {
	return c.emitter
}

func (c *client) List(ctx context.Context) []Pod {
	result := []Pod{}

	list, err := c.pods.Lister().List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Errorf("unexpected error")
		return nil
	}

	for _, obj := range list {
		labels := obj.ObjectMeta.Labels
		if labels == nil {
			// empty map, so retrieving a key fails silently
			labels = map[string]string{}
		}

		pod := Pod{
			Name:      obj.ObjectMeta.Name,
			Namespace: obj.ObjectMeta.Namespace,
			NodeName:  obj.Spec.NodeName,

			CreatedTime: obj.ObjectMeta.CreationTimestamp.Time,

			AppName:      labels["app.kubernetes.io/name"],
			AppInstance:  labels["app.kubernetes.io/instance"],
			AppVersion:   labels["app.kubernetes.io/version"],
			AppComponent: labels["app.kubernetes.io/component"],
		}

		pod.Ready = true
		for _, condition := range obj.Status.Conditions {
			if condition.Status != v1.ConditionTrue {
				pod.Ready = false
				break
			}
		}

		owner, ownerReady := c.getOwner(ctx, obj)
		pod.OwnerReady = ownerReady
		if owner != nil {
			pod.OwnerKind = owner.Kind
			pod.OwnerName = owner.Name
		}

		result = append(result, pod)
	}

	sort.Slice(result, func(i, j int) bool {
		// Sorting by something other than CreatedTime is required, because the
		// time has only second precision and it is quite likely that some
		// instances are started at the same time. And since the list is based
		// on a map, the order would be flaky.
		return result[i].Name < result[j].Name
	})

	sort.SliceStable(result, func(i, j int) bool {
		return result[j].CreatedTime.Before(result[i].CreatedTime)
	})

	return result
}

func (c *client) Run(ctx context.Context) error {
	// Kubernetes serves an utility to handle API crashes
	defer runtime.HandleCrash()

	egrp, ctx := errgroup.WithContext(ctx)
	run := func(name string, inf cache.SharedInformer) {
		egrp.Go(func() error {
			inf.Run(ctx.Done())
			return errors.Errorf("informer for %s stopped", name)
		})
	}

	run("Pods", c.pods.Informer())
	run("ReplicaSets", c.rs.Informer())
	run("StatefulSets", c.sts.Informer())
	run("Deployments", c.deploy.Informer())

	return errors.WithStack(egrp.Wait())
}
