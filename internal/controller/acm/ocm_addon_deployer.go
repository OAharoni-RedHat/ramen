// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const volsyncCRDName = "replicationsources.volsync.backube"

// ocmAddonDeployer implements AddonDeployer for OCM-only hubs.
// Instead of deploying via ManagedClusterAddOn (ACM), it checks for
// pre-installed add-ons and logs guidance if they are missing.
type ocmAddonDeployer struct {
	Client client.Client
	Log    logr.Logger
}

// NewOCMAddonDeployer creates an AddonDeployer for OCM-only hubs.
func NewOCMAddonDeployer(c client.Client, log logr.Logger) AddonDeployer {
	return &ocmAddonDeployer{
		Client: c,
		Log:    log,
	}
}

func (o *ocmAddonDeployer) DeployAddon(ctx context.Context, addonName, clusterName string) error {
	if addonName != "volsync" {
		return fmt.Errorf("unsupported addon: %s", addonName)
	}

	// Check if the VolSync CRD exists on the hub as a proxy for availability.
	// On ACM hubs, VolSync is deployed via ManagedClusterAddOn. On plain OCM hubs,
	// VolSync must be pre-installed by the user on each managed cluster.
	crd := &apiextensionsv1.CustomResourceDefinition{}

	err := o.Client.Get(ctx, types.NamespacedName{Name: volsyncCRDName}, crd)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			o.Log.Info(
				"VolSync is not installed on cluster. "+
					"On ACM hubs, VolSync is deployed automatically via ManagedClusterAddOn. "+
					"On plain OCM hubs, VolSync must be pre-installed. "+
					"See documentation for installation instructions.",
				"cluster", clusterName,
			)

			return nil
		}

		return fmt.Errorf("failed to check for VolSync CRD: %w", err)
	}

	o.Log.Info("VolSync CRD detected, assuming VolSync is available", "cluster", clusterName)

	return nil
}

// Ensure ocmAddonDeployer implements AddonDeployer.
var _ AddonDeployer = &ocmAddonDeployer{}
