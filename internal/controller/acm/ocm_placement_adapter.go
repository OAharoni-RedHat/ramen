// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clrapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ocmPlacementAdapter implements PlacementAdapter supporting only OCM Placement.
// PlacementRule is not supported on hubs without ACM.
type ocmPlacementAdapter struct {
	Client client.Client
}

// NewOCMPlacementAdapter creates a PlacementAdapter that supports only OCM Placement (no PlacementRule).
func NewOCMPlacementAdapter(c client.Client) PlacementAdapter {
	return &ocmPlacementAdapter{
		Client: c,
	}
}

func (o *ocmPlacementAdapter) GetPlacementObject(
	ctx context.Context,
	ref corev1.ObjectReference,
	namespace string,
) (client.Object, error) {
	switch ref.Kind {
	case "PlacementRule":
		return nil, fmt.Errorf(
			"PlacementRule is not supported on this hub (ACM not detected). " +
				"Please migrate to Placement (apiVersion: cluster.open-cluster-management.io/v1beta1, " +
				"kind: Placement). See Ramen documentation for migration guide")

	case "Placement", "":
		p := &clrapiv1beta1.Placement{}
		key := types.NamespacedName{Name: ref.Name, Namespace: namespace}

		if err := o.Client.Get(ctx, key, p); err != nil {
			return nil, err
		}

		return p, nil

	default:
		return nil, fmt.Errorf("unsupported placement kind: %s", ref.Kind)
	}
}

func (o *ocmPlacementAdapter) SupportsPlacementRule() bool {
	return false
}

func (o *ocmPlacementAdapter) SetupWatches(
	b *ctrl.Builder,
	_ handler.EventHandler, _ predicate.Predicate,
	placementHandler handler.EventHandler, placementPred predicate.Predicate,
) error {
	(*b).Watches(&clrapiv1beta1.Placement{}, placementHandler, builder.WithPredicates(placementPred))

	return nil
}

// Ensure ocmPlacementAdapter implements PlacementAdapter.
var _ PlacementAdapter = &ocmPlacementAdapter{}
