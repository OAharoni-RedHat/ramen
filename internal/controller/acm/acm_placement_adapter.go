// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"fmt"

	plrv1 "github.com/stolostron/multicloud-operators-placementrule/pkg/apis/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clrapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// acmPlacementAdapter implements PlacementAdapter supporting both PlacementRule
// and Placement, which is the behavior when ACM is installed.
type acmPlacementAdapter struct {
	Client client.Client
}

// NewACMPlacementAdapter creates a PlacementAdapter that supports both PlacementRule and Placement.
func NewACMPlacementAdapter(c client.Client) PlacementAdapter {
	return &acmPlacementAdapter{
		Client: c,
	}
}

func (a *acmPlacementAdapter) GetPlacementObject(
	ctx context.Context,
	ref corev1.ObjectReference,
	namespace string,
) (client.Object, error) {
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}

	switch ref.Kind {
	case "PlacementRule":
		pr := &plrv1.PlacementRule{}
		if err := a.Client.Get(ctx, key, pr); err != nil {
			return nil, err
		}

		return pr, nil

	case "Placement":
		p := &clrapiv1beta1.Placement{}
		if err := a.Client.Get(ctx, key, p); err != nil {
			return nil, err
		}

		return p, nil

	default:
		return nil, fmt.Errorf("unsupported placement kind: %s", ref.Kind)
	}
}

func (a *acmPlacementAdapter) SupportsPlacementRule() bool {
	return true
}

func (a *acmPlacementAdapter) SetupWatches(b *ctrl.Builder, h handler.EventHandler) error {
	(*b).Watches(&plrv1.PlacementRule{}, h)
	(*b).Watches(&clrapiv1beta1.Placement{}, h)

	return nil
}

// Ensure acmPlacementAdapter implements PlacementAdapter.
var _ PlacementAdapter = &acmPlacementAdapter{}
