// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ramendr/ramen/internal/controller/volsync"
)

// acmAddonDeployer implements AddonDeployer using the existing
// ManagedClusterAddOn mechanism via volsync.DeployVolSyncToCluster.
type acmAddonDeployer struct {
	Client client.Client
	Log    logr.Logger
}

// NewACMAddonDeployer creates an AddonDeployer backed by ACM ManagedClusterAddOn.
func NewACMAddonDeployer(c client.Client, log logr.Logger) AddonDeployer {
	return &acmAddonDeployer{
		Client: c,
		Log:    log,
	}
}

func (a *acmAddonDeployer) DeployAddon(ctx context.Context, addonName, clusterName string) error {
	if addonName == "volsync" {
		return volsync.DeployVolSyncToCluster(ctx, a.Client, clusterName, a.Log)
	}

	return fmt.Errorf("unsupported addon: %s", addonName)
}

// Ensure acmAddonDeployer implements AddonDeployer.
var _ AddonDeployer = &acmAddonDeployer{}
