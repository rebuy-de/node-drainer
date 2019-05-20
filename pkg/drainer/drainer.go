package drainer

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rebuy-de/node-drainer/pkg/prom"
)

type ErrNodeNotAvailable string

func (err ErrNodeNotAvailable) Error() string {
	return fmt.Sprintf("node %s not available, skipping draining...", err)
}

func IsErrNodeNotAvailable(err error) bool {
	_, ok := err.(ErrNodeNotAvailable)
	if ok {
		return true
	}

	return false
}

type Drainer struct {
	Clientset     kubernetes.Interface
	TaintInit     *corev1.Taint
	TaintShutdown *corev1.Taint
	Wait          bool
}

func NewDrainer(clientset kubernetes.Interface) *Drainer {
	tInitial := &corev1.Taint{
		Key:    "rebuy.com/initial",
		Value:  "Exists",
		Effect: "NoExecute",
	}

	tShutdown := &corev1.Taint{
		Key:    "rebuy.com/shutdown",
		Value:  "Exists",
		Effect: "NoSchedule",
	}

	return &Drainer{
		Clientset:     clientset,
		TaintInit:     tInitial,
		TaintShutdown: tShutdown,
		Wait:          true,
	}
}

func (d *Drainer) Drain(nodeName string) error {
	n := d.node(nodeName)
	if n == nil {
		return ErrNodeNotAvailable(nodeName)
	}

	log.Infof("Draining node %s", n.GetName())

	start := time.Now()

	if !d.hasShutdownTaint(n.Spec.Taints) {
		n.Spec.Taints = append(n.Spec.Taints, *d.TaintShutdown)
		_, err := d.Clientset.Core().Nodes().Update(n)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	d.evictAllPods(n)

	for d.getRemainingPodCount(n) > 0 {
		log.Debug("Remaining pod count:", d.getRemainingPodCount(n))
		time.Sleep(time.Duration(1) * time.Second)
	}
	log.Infof("Finished draining node %s", n.GetName())

	prom.M.SetLastEvictionDuration(time.Since(start).Seconds())
	return nil
}

func (d *Drainer) node(nodeName string) *corev1.Node {
	if nodeName == "" {
		log.Warn("Empty node name string, skipping...")
		return nil
	}
	var err error
	var n *corev1.Node
	var opts metav1.GetOptions
	n, err = d.Clientset.CoreV1().Nodes().Get(nodeName, opts)
	if err != nil {
		switch err.(type) {
		case *errors.StatusError:
			log.Warn(err)
			return nil
		case *url.Error:
			log.Error("Terminating due to error: " + err.Error())
			cmdutil.Exit(1)
			return nil
		}
	}
	return n
}

func (d *Drainer) hasShutdownTaint(taints []corev1.Taint) bool {
	for t := range taints {
		if taints[t].Key == d.TaintShutdown.Key && taints[t].Value == d.TaintShutdown.Value {
			return true
		}
	}
	return false
}

func (d *Drainer) evictAllPods(node *corev1.Node) {
	var (
		evictions int
		lo        metav1.ListOptions
	)

	pods, err := d.Clientset.CoreV1().Pods("").List(lo)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}

	eg, _ := errgroup.WithContext(context.Background())

	for _, pod := range pods.Items {
		if pod.Spec.NodeName != node.GetName() {
			// Pod is actually not on draining node.
			continue
		}

		// Workaround, because otherwise Go would not scope `pod` into the loop
		// and overwrite it in the next iteration.
		// See https://golang.org/doc/faq#closures_and_goroutines
		pod := pod

		eg.Go(func() error { return d.evict(pod) })
		evictions = evictions + 1
		prom.M.IncreaseEvictedPods()
	}

	err = eg.Wait()
	if err != nil {
		log.Errorf("Node eviction for node %s failed: %v", node.GetName(), err)
		log.Warnf("Calling for help by dying...")
		cmdutil.Exit(1)
	}

	log.Infof("Evicted %d Pods on Node %s", evictions, node.GetName())
}

func (d *Drainer) evict(pod corev1.Pod) error {
	var (
		maxRetries = 10
		retries    = 0

		do metav1.DeleteOptions

		l = log.WithField("Pod", pod.GetName())
	)

	if d.podHasInitToleration(pod.Spec.Tolerations) {
		// Skip critical Pods, like kube-proxy and kube-dns DaemonSets.
		l.Info("Pod has toleration and will not get drained.")
		return nil
	}

	eviction := &policyv1beta1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.GetName(),
			Namespace: pod.GetNamespace(),
		},
		DeleteOptions: &do,
	}

	for {
		err := d.Clientset.PolicyV1beta1().Evictions(eviction.GetNamespace()).Evict(eviction)
		if err == nil {
			l.Info("Eviction triggered")
			return nil
		}

		l.Errorf("Triggering eviction failed: %v", err)

		if d.Wait == true {
			time.Sleep(time.Duration(1) * time.Second)
		}

		retries++
		if retries < maxRetries {
			l.WithField("error", err).Infof("Retrying eviction ...")
			continue
		}

		log.Errorf("Triggering eviction failed permanently.")

		if pod.Status.Phase == corev1.PodUnknown {
			l.Warnf("Ignoring failed eviction, because Pod is in Unknown phase.")
			return nil
		}

		return fmt.Errorf("Triggering eviction for %s failed permanently.", eviction.GetName())
	}
}

func (d *Drainer) getRemainingPodCount(node *corev1.Node) int {
	var (
		lo          metav1.ListOptions
		podsPending int
	)

	pods, err := d.Clientset.CoreV1().Pods("").List(lo)

	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}

	for pd := range pods.Items {
		if pods.Items[pd].Spec.NodeName == node.GetName() && !d.podHasInitToleration(pods.Items[pd].Spec.Tolerations) {
			log.Debug("node and pod matched! node: ", node.GetName(), " pods: ", pods.Items[pd].GetName())
			podsPending = podsPending + 1
		}
	}

	return podsPending
}

func (d *Drainer) podHasInitToleration(tolerations []corev1.Toleration) bool {
	for t := range tolerations {
		if tolerations[t].ToleratesTaint(d.TaintInit) {
			return true
		}
	}
	return false
}
