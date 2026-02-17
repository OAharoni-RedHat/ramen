// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/volsync"
)

// acmVolSyncSecretPropagator implements VolSyncSecretPropagator using the existing
// ACM Policy chain for VolSync PSK secret distribution.
type acmVolSyncSecretPropagator struct {
	Client client.Client
}

// NewACMVolSyncSecretPropagator creates a VolSyncSecretPropagator backed by ACM policies.
func NewACMVolSyncSecretPropagator(c client.Client) VolSyncSecretPropagator {
	return &acmVolSyncSecretPropagator{
		Client: c,
	}
}

func (a *acmVolSyncSecretPropagator) PropagateSecretToClusters(
	ctx context.Context,
	sourceSecret *corev1.Secret,
	ownerObject metav1.Object,
	destClusters []string,
	destSecretName, destSecretNamespace string,
) error {
	log := ctrl.Log.WithName("acm-volsync-propagator")

	return volsync.PropagateSecretToClusters(
		ctx, a.Client, sourceSecret, ownerObject,
		destClusters, destSecretName, destSecretNamespace, log,
	)
}

func (a *acmVolSyncSecretPropagator) CleanupSecretPropagation(
	ctx context.Context,
	ownerObject metav1.Object,
) error {
	log := ctrl.Log.WithName("acm-volsync-propagator")

	return volsync.CleanupSecretPropagation(ctx, a.Client, ownerObject, log)
}

// Ensure acmVolSyncSecretPropagator implements VolSyncSecretPropagator.
var _ VolSyncSecretPropagator = &acmVolSyncSecretPropagator{}
