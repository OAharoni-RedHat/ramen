// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package acm

import (
	"context"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var detectLog = ctrl.Log.WithName("acm-detect")

const acmPolicyCRDName = "policies.policy.open-cluster-management.io"

// DetectACM checks whether ACM is installed on the hub cluster by looking for the
// ACM Policy CRD. Returns (true, nil) if ACM is detected, (false, nil) if not,
// or (false, err) on unexpected errors.
func DetectACM(ctx context.Context, reader client.Reader) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}

	err := reader.Get(ctx, types.NamespacedName{Name: acmPolicyCRDName}, crd)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			detectLog.Info("ACM not detected on hub cluster — OCM-native backends will be used")

			return false, nil
		}

		return false, err
	}

	detectLog.Info("ACM detected on hub cluster — ACM backends will be used")

	return true, nil
}
