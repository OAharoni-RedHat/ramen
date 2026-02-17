// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/util"
)

// ocmVolSyncSecretPropagator implements VolSyncSecretPropagator using OCM ManifestWork
// to distribute VolSync PSK secrets, replacing the ACM Policy chain.
type ocmVolSyncSecretPropagator struct {
	Client client.Client
	Log    logr.Logger
}

// NewOCMVolSyncSecretPropagator creates a VolSyncSecretPropagator backed by ManifestWork.
func NewOCMVolSyncSecretPropagator(c client.Client, log logr.Logger) VolSyncSecretPropagator {
	return &ocmVolSyncSecretPropagator{
		Client: c,
		Log:    log,
	}
}

func (o *ocmVolSyncSecretPropagator) PropagateSecretToClusters(
	ctx context.Context,
	sourceSecret *corev1.Secret,
	ownerObject metav1.Object,
	destClusters []string,
	destSecretName, destSecretNamespace string,
) error {
	for _, cluster := range destClusters {
		o.Log.Info("Propagating VolSync PSK secret via ManifestWork",
			"secret", destSecretName, "cluster", cluster)

		targetSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      destSecretName,
				Namespace: destSecretNamespace,
			},
			Data: sourceSecret.Data,
		}

		manifest, err := generateManifest(targetSecret)
		if err != nil {
			return fmt.Errorf("failed to generate manifest for VolSync secret: %w", err)
		}

		mwName := volSyncSecretManifestWorkName(destSecretName)

		mw := &ocmworkv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mwName,
				Namespace: cluster,
				Labels: map[string]string{
					util.CreatedByRamenLabel:     "true",
					volSyncPSKOwnerLabel:         ownerObject.GetName(),
					volSyncPSKOwnerNSLabel:       ownerObject.GetNamespace(),
					volSyncPSKSecretNSLabel:      destSecretNamespace,
				},
			},
			Spec: ocmworkv1.ManifestWorkSpec{
				Workload: ocmworkv1.ManifestsTemplate{
					Manifests: []ocmworkv1.Manifest{*manifest},
				},
			},
		}

		if err := o.createOrUpdateManifestWork(ctx, mw, cluster); err != nil {
			return fmt.Errorf("failed to create/update VolSync secret ManifestWork for cluster %s: %w", cluster, err)
		}
	}

	return nil
}

func (o *ocmVolSyncSecretPropagator) CleanupSecretPropagation(
	ctx context.Context,
	ownerObject metav1.Object,
) error {
	o.Log.Info("Cleaning up VolSync PSK secret ManifestWorks",
		"owner", ownerObject.GetName(), "namespace", ownerObject.GetNamespace())

	mwList := &ocmworkv1.ManifestWorkList{}
	if err := o.Client.List(ctx, mwList, client.MatchingLabels{
		volSyncPSKOwnerLabel:   ownerObject.GetName(),
		volSyncPSKOwnerNSLabel: ownerObject.GetNamespace(),
	}); err != nil {
		return fmt.Errorf("failed to list VolSync PSK ManifestWorks: %w", err)
	}

	for i := range mwList.Items {
		mw := &mwList.Items[i]

		o.Log.Info("Deleting VolSync PSK ManifestWork", "name", mw.Name, "namespace", mw.Namespace)

		if err := o.Client.Delete(ctx, mw); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete VolSync PSK ManifestWork %s/%s: %w", mw.Namespace, mw.Name, err)
		}
	}

	return nil
}

func (o *ocmVolSyncSecretPropagator) createOrUpdateManifestWork(
	ctx context.Context, mw *ocmworkv1.ManifestWork, clusterNamespace string,
) error {
	foundMW := &ocmworkv1.ManifestWork{}
	key := types.NamespacedName{Name: mw.Name, Namespace: clusterNamespace}

	err := o.Client.Get(ctx, key, foundMW)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch ManifestWork %s: %w", key, err)
		}

		o.Log.Info("Creating VolSync PSK ManifestWork", "name", mw.Name, "cluster", clusterNamespace)

		return o.Client.Create(ctx, mw)
	}

	o.Log.Info("Updating VolSync PSK ManifestWork", "name", mw.Name, "cluster", clusterNamespace)

	foundMW.Spec = mw.Spec
	foundMW.Labels = mw.Labels

	return o.Client.Update(ctx, foundMW)
}

const (
	volSyncPSKOwnerLabel    = "ramendr.openshift.io/volsync-psk-owner"
	volSyncPSKOwnerNSLabel  = "ramendr.openshift.io/volsync-psk-owner-ns"
	volSyncPSKSecretNSLabel = "ramendr.openshift.io/volsync-psk-secret-ns"
)

// volSyncSecretManifestWorkName generates a ManifestWork name for a VolSync PSK secret.
func volSyncSecretManifestWorkName(destSecretName string) string {
	return fmt.Sprintf("%s-volsync-psk-mw", destSecretName)
}

// Ensure ocmVolSyncSecretPropagator implements VolSyncSecretPropagator.
var _ VolSyncSecretPropagator = &ocmVolSyncSecretPropagator{}
