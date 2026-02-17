// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/ramendr/ramen/internal/controller/util"
)

// SecretPropagator distributes secrets from the hub to managed clusters.
// ACM backend uses Policy/PlacementRule/PlacementBinding chain.
// OCM backend uses ManifestWork to deploy secrets directly.
type SecretPropagator interface {
	// PropagateSecretToCluster propagates a secret from the hub to a managed cluster.
	PropagateSecretToCluster(
		secretName, sourceNamespace, clusterName, targetNamespace string,
		format util.TargetSecretFormat,
		veleroSecretKeyName string,
	) error

	// RemoveSecretFromCluster removes a previously propagated secret from a managed cluster.
	RemoveSecretFromCluster(
		secretName, clusterName, namespace string,
		format util.TargetSecretFormat,
	) error

	// Cleanup removes all secret propagation resources for the given secret.
	Cleanup(secretName, sourceNamespace string, format util.TargetSecretFormat) error
}

// VolSyncSecretPropagator distributes VolSync PSK secrets to managed clusters.
// ACM backend uses Policy/PlacementRule/PlacementBinding chain.
// OCM backend uses ManifestWork.
type VolSyncSecretPropagator interface {
	// PropagateSecretToClusters propagates a VolSync PSK secret to the specified clusters.
	PropagateSecretToClusters(
		ctx context.Context,
		sourceSecret *corev1.Secret,
		ownerObject metav1.Object,
		destClusters []string,
		destSecretName, destSecretNamespace string,
	) error

	// CleanupSecretPropagation removes VolSync PSK secret propagation resources.
	CleanupSecretPropagation(
		ctx context.Context,
		ownerObject metav1.Object,
	) error
}

// AddonDeployer deploys add-on operators to managed clusters.
// ACM backend uses ManagedClusterAddOn.
// OCM backend checks for pre-installed operators and logs guidance.
type AddonDeployer interface {
	// DeployAddon deploys the named addon to the specified managed cluster.
	DeployAddon(ctx context.Context, addonName, clusterName string) error
}

// PlacementAdapter handles PlacementRule (ACM) vs Placement (OCM) differences.
// ACM backend supports both PlacementRule and Placement.
// OCM backend supports only Placement.
type PlacementAdapter interface {
	// GetPlacementObject retrieves a placement object (PlacementRule or Placement) by reference.
	GetPlacementObject(
		ctx context.Context,
		ref corev1.ObjectReference,
		namespace string,
	) (client.Object, error)

	// SupportsPlacementRule returns true if the backend supports PlacementRule resources.
	SupportsPlacementRule() bool

	// SetupWatches configures the controller watches for placement-related resources.
	SetupWatches(builder *ctrl.Builder, handler handler.EventHandler) error
}
