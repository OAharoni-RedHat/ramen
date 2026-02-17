// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/util"
)

// ocmSecretPropagator implements SecretPropagator using OCM ManifestWork
// to deploy secrets directly to managed clusters, bypassing ACM Policy chains.
type ocmSecretPropagator struct {
	Client    client.Client
	APIReader client.Reader
	Ctx       context.Context
	Log       logr.Logger
}

// NewOCMSecretPropagator creates a SecretPropagator backed by OCM ManifestWork.
func NewOCMSecretPropagator(
	c client.Client, apiReader client.Reader, ctx context.Context, log logr.Logger,
) SecretPropagator {
	return &ocmSecretPropagator{
		Client:    c,
		APIReader: apiReader,
		Ctx:       ctx,
		Log:       log,
	}
}

func (o *ocmSecretPropagator) PropagateSecretToCluster(
	secretName, sourceNamespace, clusterName, targetNamespace string,
	format util.TargetSecretFormat,
	veleroSecretKeyName string,
) error {
	o.Log.Info("Propagating secret via ManifestWork",
		"secret", secretName, "cluster", clusterName, "format", format)

	sourceSecret := &corev1.Secret{}
	if err := o.Client.Get(o.Ctx, types.NamespacedName{
		Name: secretName, Namespace: sourceNamespace,
	}, sourceSecret); err != nil {
		return fmt.Errorf("failed to get source secret %s/%s: %w", sourceNamespace, secretName, err)
	}

	targetSecret, err := o.buildTargetSecret(sourceSecret, secretName, targetNamespace, format, veleroSecretKeyName)
	if err != nil {
		return err
	}

	mwName := secretManifestWorkName(secretName, format)

	return o.createOrUpdateSecretManifestWork(mwName, clusterName, targetSecret)
}

func (o *ocmSecretPropagator) RemoveSecretFromCluster(
	secretName, clusterName, namespace string,
	format util.TargetSecretFormat,
) error {
	o.Log.Info("Removing secret ManifestWork",
		"secret", secretName, "cluster", clusterName, "format", format)

	mwName := secretManifestWorkName(secretName, format)

	return o.deleteManifestWork(mwName, clusterName)
}

func (o *ocmSecretPropagator) Cleanup(secretName, sourceNamespace string, format util.TargetSecretFormat) error {
	o.Log.Info("Cleaning up secret ManifestWork resources",
		"secret", secretName, "format", format)

	mwName := secretManifestWorkName(secretName, format)

	mwList := &ocmworkv1.ManifestWorkList{}
	if err := o.Client.List(o.Ctx, mwList); err != nil {
		return fmt.Errorf("failed to list ManifestWorks for cleanup: %w", err)
	}

	for i := range mwList.Items {
		mw := &mwList.Items[i]
		if mw.Name == mwName {
			if err := o.deleteManifestWork(mw.Name, mw.Namespace); err != nil {
				return err
			}
		}
	}

	return nil
}

func (o *ocmSecretPropagator) buildTargetSecret(
	sourceSecret *corev1.Secret,
	secretName, targetNamespace string,
	format util.TargetSecretFormat,
	veleroSecretKeyName string,
) (*corev1.Secret, error) {
	switch format {
	case util.SecretFormatRamen:
		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: targetNamespace,
			},
			Data: sourceSecret.Data,
		}, nil

	case util.SecretFormatVelero:
		accessKeyID := string(sourceSecret.Data["AWS_ACCESS_KEY_ID"])
		secretAccessKey := string(sourceSecret.Data["AWS_SECRET_ACCESS_KEY"])

		credentialFileContent := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n",
			accessKeyID, secretAccessKey)

		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      util.GenerateVeleroSecretName(secretName),
				Namespace: veleroSecretKeyName,
			},
			Data: map[string][]byte{
				"cloud": []byte(credentialFileContent),
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported secret format: %s", format)
	}
}

func (o *ocmSecretPropagator) createOrUpdateSecretManifestWork(
	mwName, clusterNamespace string,
	targetSecret *corev1.Secret,
) error {
	manifest, err := generateManifest(targetSecret)
	if err != nil {
		return fmt.Errorf("failed to generate manifest for secret: %w", err)
	}

	mw := &ocmworkv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mwName,
			Namespace: clusterNamespace,
			Labels: map[string]string{
				util.CreatedByRamenLabel: "true",
			},
		},
		Spec: ocmworkv1.ManifestWorkSpec{
			Workload: ocmworkv1.ManifestsTemplate{
				Manifests: []ocmworkv1.Manifest{*manifest},
			},
		},
	}

	foundMW := &ocmworkv1.ManifestWork{}
	key := types.NamespacedName{Name: mwName, Namespace: clusterNamespace}

	err = o.Client.Get(o.Ctx, key, foundMW)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch ManifestWork %s: %w", key, err)
		}

		o.Log.Info("Creating secret ManifestWork", "name", mwName, "cluster", clusterNamespace)

		return o.Client.Create(o.Ctx, mw)
	}

	if !reflect.DeepEqual(foundMW.Spec, mw.Spec) {
		o.Log.Info("Updating secret ManifestWork", "name", mwName, "cluster", clusterNamespace)

		foundMW.Spec = mw.Spec

		return o.Client.Update(o.Ctx, foundMW)
	}

	return nil
}

func (o *ocmSecretPropagator) deleteManifestWork(mwName, mwNamespace string) error {
	mw := &ocmworkv1.ManifestWork{}

	err := o.Client.Get(o.Ctx, types.NamespacedName{Name: mwName, Namespace: mwNamespace}, mw)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to retrieve ManifestWork %s/%s: %w", mwNamespace, mwName, err)
	}

	o.Log.Info("Deleting secret ManifestWork", "name", mwName, "namespace", mwNamespace)

	if err := o.Client.Delete(o.Ctx, mw); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ManifestWork %s/%s: %w", mwNamespace, mwName, err)
	}

	return nil
}

// secretManifestWorkName generates a unique ManifestWork name for a secret.
func secretManifestWorkName(secretName string, format util.TargetSecretFormat) string {
	return fmt.Sprintf("%s-%s-secret-mw", secretName, format)
}

// generateManifest marshals a Kubernetes object into a ManifestWork Manifest.
func generateManifest(obj interface{}) (*ocmworkv1.Manifest, error) {
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	manifest := &ocmworkv1.Manifest{}
	manifest.RawExtension = runtime.RawExtension{Raw: objJSON}

	return manifest, nil
}

// Ensure ocmSecretPropagator implements SecretPropagator.
var _ SecretPropagator = &ocmSecretPropagator{}
