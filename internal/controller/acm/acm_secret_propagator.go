// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/util"
)

// acmSecretPropagator implements SecretPropagator using the existing ACM
// Policy/PlacementRule/PlacementBinding chain via util.SecretsUtil.
type acmSecretPropagator struct {
	Client    client.Client
	APIReader client.Reader
	Ctx       context.Context
	Log       logr.Logger
}

// NewACMSecretPropagator creates a SecretPropagator backed by ACM policies.
func NewACMSecretPropagator(
	c client.Client, apiReader client.Reader, ctx context.Context, log logr.Logger,
) SecretPropagator {
	return &acmSecretPropagator{
		Client:    c,
		APIReader: apiReader,
		Ctx:       ctx,
		Log:       log,
	}
}

func (a *acmSecretPropagator) PropagateSecretToCluster(
	secretName, sourceNamespace, clusterName, targetNamespace string,
	format util.TargetSecretFormat,
	veleroSecretKeyName string,
) error {
	secretsUtil := &util.SecretsUtil{
		Client:    a.Client,
		APIReader: a.APIReader,
		Ctx:       a.Ctx,
		Log:       a.Log,
	}

	return secretsUtil.AddSecretToCluster(secretName, clusterName, sourceNamespace, targetNamespace, format, veleroSecretKeyName)
}

func (a *acmSecretPropagator) RemoveSecretFromCluster(
	secretName, clusterName, namespace string,
	format util.TargetSecretFormat,
) error {
	secretsUtil := &util.SecretsUtil{
		Client:    a.Client,
		APIReader: a.APIReader,
		Ctx:       a.Ctx,
		Log:       a.Log,
	}

	return secretsUtil.RemoveSecretFromCluster(secretName, clusterName, namespace, format)
}

func (a *acmSecretPropagator) Cleanup(secretName, sourceNamespace string, format util.TargetSecretFormat) error {
	// Cleanup is handled by the delete path in the existing ACM policy code.
	return nil
}
