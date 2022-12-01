package pod

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rebuy-de/rebuy-go-sdk/v3/pkg/logutil"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type PDBReadyReason struct {
	CanDecrement bool   `logfield:"pdp-owner-ready-can-decrement"`
	Short        string `logfield:"pdp-owner-ready-short"`
	Reason       string `logfield:"-"`
}

func (c *client) getPDP(ctx context.Context, pod *v1.Pod) PDBReadyReason {
	if pod.ObjectMeta.DeletionTimestamp != nil {
		return PDBReadyReason{
			CanDecrement: false,
			Short:        "AlreadyTerminating",
			Reason:       "Pod is already in termination process.",
		}
	}

	fnerr := func(err error, short string) PDBReadyReason {
		or := PDBReadyReason{
			CanDecrement: false,
			Short:        short,
			Reason:       fmt.Sprintf("%v", err),
		}
		logutil.Get(ctx).
			WithError(errors.WithStack(err)).
			WithFields(logutil.FromStruct(or)).
			Error("failed to determine PDP readiness")
		return or
	}

	pdbs, err := c.pdb.Lister().List(labels.Everything())
	if err != nil {
		return fnerr(err, "PDBGetError")
	}

	matches := 0

	for _, pdb := range pdbs {
		selector, err := meta_v1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			return fnerr(err, "PDBGetSelector")
		}

		if !selector.Matches(labels.Set(pod.ObjectMeta.Labels)) {
			continue
		}

		matches++

		if pdb.Status.DisruptionsAllowed == 0 {
			return PDBReadyReason{
				CanDecrement: false,
				Short:        "NoDisruptionsAllowed",
				Reason: fmt.Sprintf("%d Pods ready, but must have at least %d",
					pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy),
			}
		}
	}

	if matches > 0 {
		return PDBReadyReason{
			CanDecrement: true,
			Short:        "PDBOK",
			Reason:       fmt.Sprintf("All %d matching PDBs allow disruptions.", matches),
		}
	}

	return PDBReadyReason{
		CanDecrement: true,
		Short:        "NoPDB",
		Reason:       "No PDB found for this pod.",
	}
}
