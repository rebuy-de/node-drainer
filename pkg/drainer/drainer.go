package drainer

import (
	"fmt"
	"net/url"
	"time"

	"github.com/rebuy-de/rebuy-go-sdk/cmdutil"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Drainer struct {
	Clientset     kubernetes.Interface
	TaintInit     *v1.Taint
	TaintShutdown *v1.Taint
	Wait          bool
}

func NewDrainer(clientset kubernetes.Interface) *Drainer {
	tInitial := &v1.Taint{
		Key:    "rebuy.com/initial",
		Value:  "Exists",
		Effect: "NoExecute",
	}

	tShutdown := &v1.Taint{
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
		log.Info("node " + nodeName + " not available, skipping draining...")
		return fmt.Errorf("node " + nodeName + " not available, skipping draining...")
	}

	log.Info("draining: ", n.GetName())

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
		log.Debug("remaining pod count:", d.getRemainingPodCount(n))
		time.Sleep(time.Duration(1) * time.Second)
	}
	log.Info("finished draining: ", n.GetName())
	return nil
}

func (d *Drainer) node(nodeName string) *v1.Node {
	if nodeName == "" {
		log.Warn("empty node name string, skipping...")
		return nil
	}
	var err error
	var n *v1.Node
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

func (d *Drainer) hasShutdownTaint(taints []v1.Taint) bool {
	for t := range taints {
		if taints[t].Key == d.TaintShutdown.Key && taints[t].Value == d.TaintShutdown.Value {
			return true
		}
	}
	return false
}

func (d *Drainer) evictAllPods(node *v1.Node) {
	var (
		evictions int
		lo        metav1.ListOptions
		do        metav1.DeleteOptions
	)

	pods, err := d.Clientset.CoreV1().Pods("").List(lo)
	if err != nil {
		log.Error(err)
		cmdutil.Exit(1)
	}
	for pd := range pods.Items {

		if pods.Items[pd].Spec.NodeName == node.GetName() && !d.podHasInitToleration(pods.Items[pd].Spec.Tolerations) {

			eviction := &policyv1beta1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pods.Items[pd].GetName(),
					Namespace: pods.Items[pd].GetNamespace(),
				},
				DeleteOptions: &do,
			}
			go d.evict(eviction)
			evictions = evictions + 1
		}
	}

	if evictions == 0 {
		log.Info("no pods to evict on node: ", node.GetName())
	}
}

func (d *Drainer) evict(eviction *policyv1beta1.Eviction) {
	var (
		maxRetries = 10
		retries    = 0
	)
	for d.Clientset.PolicyV1beta1().Evictions(eviction.GetNamespace()).Evict(eviction) != nil {
		log.Debug("failed to trigger eviction for " + eviction.GetName() + ", retrying...")
		if d.Wait == true {
			time.Sleep(time.Duration(1) * time.Second)
		}
		retries++
		if retries >= maxRetries {
			cmdutil.Exit(1)
		}
	}
	log.Info("eviction triggered: " + eviction.GetName())
}

func (d *Drainer) getRemainingPodCount(node *v1.Node) int {
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

func (d *Drainer) podHasInitToleration(tolerations []v1.Toleration) bool {
	for t := range tolerations {
		if tolerations[t].ToleratesTaint(d.TaintInit) {
			return true
		}
	}
	return false
}
