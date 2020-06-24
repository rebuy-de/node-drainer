package pod

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v2/pkg/logutil"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OwnerReadyReason struct {
	CanDecrement bool   `logfield:"pod-owner-ready-can-decrement"`
	Short        string `logfield:"pod-owner-ready-short"`
	Reason       string `logfield:"-"`
}

func (c *client) getOwner(ctx context.Context, pod *v1.Pod) (*meta_v1.OwnerReference, OwnerReadyReason) {
	owner := meta_v1.GetControllerOf(pod)

	if owner == nil {
		return nil, OwnerReadyReason{
			CanDecrement: true, Short: "NoOwner",
			Reason: "Pods with no Owner are always allowed",
		}
	}

	fnerr := func(err error, short string) OwnerReadyReason {
		or := OwnerReadyReason{
			CanDecrement: false,
			Short:        short,
			Reason:       fmt.Sprintf("%v", err),
		}
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			WithFields(logutil.FromStruct(or)).
			Error("failed to determine owner readiness")
		return or
	}

	switch owner.Kind {
	default:
		logutil.Get(ctx).
			WithError(errors.Errorf("unknown owner kind %s", owner.Kind)).
			Error("failed to get owner readiness")
		return owner, OwnerReadyReason{
			CanDecrement: true, Short: "UnknownKind",
			Reason: "Owner kind is unknown",
		}

	case "Node":
		return owner, OwnerReadyReason{
			CanDecrement: true, Short: "IsNodePod",
			Reason: "Node Pods are always allowed",
		}

	case "DaemonSet":
		return owner, OwnerReadyReason{
			CanDecrement: true, Short: "IsDaemonSetPod",
			Reason: "DaemonSet Pods are always allowed",
		}

	case "StatefulSet":
		sts, err := c.sts.Lister().StatefulSets(pod.ObjectMeta.Namespace).Get(owner.Name)
		if err != nil {
			return owner, fnerr(err, "StatefulSetGetError")
		}

		return owner, c.getOwnerReadyFromReplicas(owner.Kind,
			sts.Spec.Replicas, sts.Status.ReadyReplicas)

	case "ReplicaSet":
		rs, err := c.rs.Lister().ReplicaSets(pod.ObjectMeta.Namespace).Get(owner.Name)
		if err != nil {
			return owner, fnerr(err, "ReplicaSetGetError")
		}

		parent := meta_v1.GetControllerOf(rs)
		if parent == nil {
			return owner, c.getOwnerReadyFromReplicas(owner.Kind,
				rs.Spec.Replicas, rs.Status.AvailableReplicas)
		}

		deploy, err := c.deploy.Lister().Deployments(rs.ObjectMeta.Namespace).Get(parent.Name)
		if err != nil {
			return parent, fnerr(err, "DeploymentGetError")
		}

		return parent, c.getOwnerReadyFromReplicas(parent.Kind,
			deploy.Spec.Replicas, deploy.Status.AvailableReplicas)

	case "Job":
		return owner, OwnerReadyReason{
			CanDecrement: true, Short: "IsJobPod",
			Reason: "Job Pods are always allowed",
		}

	}
}

func (c *client) getOwnerReadyFromReplicas(kind string, specReplicas *int32, haveReplicas int32) OwnerReadyReason {
	wantReplicas := int32(1)
	if specReplicas != nil {
		wantReplicas = *specReplicas
	}

	if wantReplicas <= 1 {
		return OwnerReadyReason{
			CanDecrement: true,
			Short:        fmt.Sprintf("%sSingle", kind),
			Reason:       fmt.Sprintf("%s only wants one replica", kind),
		}
	}

	if haveReplicas < wantReplicas {
		return OwnerReadyReason{
			CanDecrement: false,
			Short:        fmt.Sprintf("%sUnready", kind),
			Reason: fmt.Sprintf("%s has only %d of %d available pods",
				kind, haveReplicas, wantReplicas),
		}
	}

	return OwnerReadyReason{
		CanDecrement: true,
		Short:        fmt.Sprintf("%sOK", kind),
		Reason:       fmt.Sprintf("%s is healthy with %d pods", kind, haveReplicas),
	}
}
